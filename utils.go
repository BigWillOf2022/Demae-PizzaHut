package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/WiiLink24/nwc24"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora/v4"
	"log"
	"net/http"
	"os"
	"strconv"
)

const QueryDiscordID = `SELECT "user".discord_id FROM "user" WHERE "user".wii_id = $1 LIMIT 1`

func NewResponse(r *http.Request, w *http.ResponseWriter, xmlType XMLType) *Response {
	wiiNumber, err := strconv.ParseUint(r.Header.Get("X-WiiNo"), 10, 64)
	if err != nil {
		// Failed to parse Wii Number or invalid integer
		(*w).WriteHeader(http.StatusBadRequest)
		return nil
	}

	number := nwc24.LoadWiiNumber(wiiNumber)
	if !number.CheckWiiNumber() {
		// Bad Wii Number
		(*w).WriteHeader(http.StatusBadRequest)
		return nil
	}

	return &Response{
		ResponseFields: KVFieldWChildren{
			XMLName: xml.Name{Local: "response"},
			Value:   nil,
		},
		wiiNumber:           number,
		request:             r,
		writer:              w,
		isMultipleRootNodes: xmlType == 1,
	}
}

func (r *Response) GetHollywoodId() string {
	return strconv.Itoa(int(r.wiiNumber.GetHollywoodID()))
}

// AddCustomType adds a given key by name to a specified structure.
func (r *Response) AddCustomType(customType any) {
	k, ok := r.ResponseFields.(KVFieldWChildren)
	if ok {
		k.Value = append(k.Value, customType)
		r.ResponseFields = k
		return
	}

	// Now check if the fields is an array of any.
	array, ok := r.ResponseFields.([]any)
	if ok {
		r.ResponseFields = append(r.ResponseFields.([]any), array)
	}
}

// AddKVNode adds a given key by name to a specified value, such as <key>value</key>.
func (r *Response) AddKVNode(key string, value string) {
	k, ok := r.ResponseFields.(KVFieldWChildren)
	if !ok {
		return
	}

	k.Value = append(k.Value, KVField{
		XMLName: xml.Name{Local: key},
		Value:   value,
	})

	r.ResponseFields = k
}

// AddKVWChildNode adds a given key by name to a specified value, such as <key><child>...</child></key>.
func (r *Response) AddKVWChildNode(key string, value any) {
	k, ok := r.ResponseFields.(KVFieldWChildren)
	if !ok {
		return
	}

	k.Value = append(k.Value, KVFieldWChildren{
		XMLName: xml.Name{Local: key},
		Value:   []any{value},
	})

	r.ResponseFields = k
}

func (r *Response) toXML() (string, error) {
	var contents string

	if r.isMultipleRootNodes {
		var temp []byte
		var err error
		array, ok := r.ResponseFields.([]any)
		if ok {
			for _, a := range array {
				temp, err = xml.MarshalIndent(a, "", "  ")
				if err != nil {
					return "", err
				}

				contents += string(temp) + "\n"
			}
		} else {
			temp, err = xml.MarshalIndent(r.ResponseFields, "", "  ")
			if err != nil {
				return "", err
			}

			contents += string(temp) + "\n"
		}

		// Now the version and API tags
		version, apiStatus := GenerateVersionAndAPIStatus()
		temp, err = xml.MarshalIndent(version, "", "  ")
		if err != nil {
			return "", err
		}

		contents += string(temp) + "\n"

		temp, err = xml.MarshalIndent(apiStatus, "", "  ")
		if err != nil {
			return "", err
		}

		contents += string(temp)
	} else {
		version, apiStatus := GenerateVersionAndAPIStatus()
		r.AddCustomType(version)
		r.AddCustomType(apiStatus)
		temp, err := xml.MarshalIndent(r.ResponseFields, "", "  ")
		if err != nil {
			return "", err
		}

		contents += string(temp)
	}

	return contents, nil
}

func GenerateVersionAndAPIStatus() (*KVField, *KVFieldWChildren) {
	version := KVField{
		XMLName: xml.Name{Local: "version"},
		Value:   "1",
	}

	apiStatus := KVFieldWChildren{
		XMLName: xml.Name{Local: "apiStatus"},
		Value: []any{
			KVField{
				XMLName: xml.Name{Local: "code"},
				Value:   "0",
			},
		},
	}

	return &version, &apiStatus
}

// BoolToInt converts a boolean value to an integer.
// This is needed because Nintendo wants the integer, not the string literal.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func PostDiscordWebhook(title, message, url string, color int) {
	theMap := map[string]any{
		"content": nil,
		"embeds": []map[string]any{
			{
				"title":       title,
				"description": message,
				"color":       color,
			},
		},
	}

	jsonData, _ := json.Marshal(theMap)
	_, _ = http.Post(url, "application/json", bytes.NewBuffer(jsonData))
}

// ReportError helps make errors nicer. First it logs the error to Sentry,
// then writes a response for the server to send.
func (r *Response) ReportError(err error, code int) {
	var discordId string
	row := pool.QueryRow(context.Background(), QueryDiscordID, r.wiiNumber.GetHollywoodID())
	_err := row.Scan(&discordId)
	if _err != nil {
		// We assume Discord ID doesn't exist because we will get an error elsewhere if the db is down.
		// UUID's are generated for each error case, so we have a unique identifier
		discordId = fmt.Sprintf("Not Registered: %s", uuid.New().String())
	}

	// Write the JSON Dominos sent us to the system.
	_ = os.WriteFile(fmt.Sprintf("errors/%s_%s.json", r.request.URL.Path, discordId), r.dominos.GetResponse(), 0664)

	sentry.WithScope(func(s *sentry.Scope) {
		s.SetTag("Discord ID", discordId)
		sentry.CaptureException(err)
	})

	log.Printf("An error has occurred: %s", aurora.Red(err.Error()))

	errorString := fmt.Sprintf("%s\nWii ID: %d\nDiscord ID: %s", err.Error(), r.wiiNumber.GetHollywoodID(), discordId)
	PostDiscordWebhook("An error has occurred in Demae Domino's!", errorString, config.ErrorWebhook, 16711711)

	// Write response
	r.hasError = true
	http.Error(*r.writer, err.Error(), code)
}

func printError(w http.ResponseWriter, reason string, code int) {
	http.Error(w, reason, code)
	log.Print("Failed to handle request: ", aurora.Red(reason))
}