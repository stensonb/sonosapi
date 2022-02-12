package sonosapi

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

const soapEnvelopeNS = "http://schemas.xmlsoap.org/soap/envelope/"
const soapEncodingNS = "http://schemas.xmlsoap.org/soap/encoding/"

type soapRequest struct {
	XMLName          xml.Name `xml:"s:Envelope"`
	XMLNsSoap        string   `xml:"xmlns:s,attr"`
	XMLEncodingStyle string   `xml:"s:encodingStyle,attr"`
	Body             soapBody
}

type soapBody struct {
	XMLName xml.Name `xml:"s:Body"`
	Payload interface{}
}

type soapResponse struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Body    *soapResponseBody
}

type soapResponseBody struct {
	XMLName xml.Name    `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Fault   *soapFault  `xml:",omitempty"`
	Content interface{} `xml:",omitempty"`
}

func (s *soapResponseBody) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	ignoreEnd := false

	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return nil
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Space == soapEnvelopeNS && elem.Name.Local == "Fault" {
				fault := soapFault{}
				err := decoder.DecodeElement(&fault, &elem)
				if err != nil {
					return fmt.Errorf("decode error")
				}
				s.Fault = &fault
				continue
			}

			var err error

			switch elem.Name.Space {
			case "urn:schemas-upnp-org:service:RenderingControl:1":
				switch elem.Name.Local {
				case "SetVolumeResponse":
					ignoreEnd = true
					break
				case "GetVolumeResponse":
					content := getVolumeResponse{}
					err = decoder.DecodeElement(&content, &elem)
					s.Content = content
					break
				}
				break
			case "urn:schemas-upnp-org:service:AVTransport:1":
				switch elem.Name.Local {
				case "PauseResponse", "PlayResponse", "SetAVTransportURIResponse", "SeekResponse":
					ignoreEnd = true
					break
				case "GetTransportInfoResponse":
					content := getPlaybackStateResponse{}
					err = decoder.DecodeElement(&content, &elem)
					s.Content = content
					break
				case "GetPositionInfoResponse":
					content := getPositionInfoResponse{}
					err = decoder.DecodeElement(&content, &elem)
					s.Content = content
					break
				case "GetMediaInfoResponse":
					content := getMediaInfoResponse{}
					err = decoder.DecodeElement(&content, &elem)
					s.Content = content
					break
				}
				break
			default:
				fmt.Printf("Unknown Payload: '%s' - '%s'\n", elem.Name.Space, elem.Name.Local)
				ignoreEnd = true
			}

			if err != nil {
				return fmt.Errorf("decode error")
			}

		case xml.EndElement:
			if elem.Name.Space == soapEnvelopeNS && elem.Name.Local == "Body" {
				return nil
			} else if ignoreEnd {
				ignoreEnd = false
			} else {
				return fmt.Errorf("unknown end element: %s", elem.Name)
			}
		}
	}
}

type soapFault struct {
	// XMLName is the serialized name of this object.
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`

	Code   string `xml:"faultcode,omitempty"`
	String string `xml:"faultstring,omitempty"`
	Actor  string `xml:"faultactor,omitempty"`

	// DetailInternal is a handle to the internal fault detail type. Do not directly access;
	// this is made public only to allow for XML deserialization.
	// Use the Detail() method instead.
	DetailInternal *soapFaultDetail `xml:"detail,omitempty"`
}

type soapFaultDetail struct {
	Content interface{} `xml:",omitempty"`
}

func (device *sonosDevice) deviceRequest(suffix string, namespace string, action string, payload interface{}) (*soapResponse, error) {
	url := fmt.Sprintf("%s/%s", device.baseURL.String(), suffix)
	aRequest := soapRequest{
		XMLNsSoap:        soapEnvelopeNS,
		XMLEncodingStyle: soapEncodingNS,
		Body: soapBody{
			Payload: payload,
		},
	}

	marshalled, err := xml.MarshalIndent(aRequest, "", "\t")
	if err != nil {
		return nil, err
	}

	buffer := bytes.NewBuffer(marshalled)

	client := http.Client{}
	request, err := http.NewRequest(http.MethodPost, url, buffer)
	if err != nil {
		return nil, fmt.Errorf("unable to construct request: %s", err)
	}

	request.Header.Set("soapaction", fmt.Sprintf("%s#%s", namespace, action))

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("unable to make request: %s", err)
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failure: %d", response.StatusCode)
	}

	dataBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response: %s", err)
	}

	parsedResponse := soapResponse{}

	err = xml.Unmarshal(dataBytes, &parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("could not parse response: %s", err)
	}

	return &parsedResponse, nil
}
