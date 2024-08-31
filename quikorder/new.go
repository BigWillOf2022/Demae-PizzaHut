package quikorder //This program is a work in progress

import (
	"net/http"
)

type Country string

const (
	USA    Country = "UNITED_STATES"
	CANADA Country = "CANADA"
)

var apiURLS = map[Country]string{
	USA:    "https://pizzahut.com",
	CANADA: "https://pizzahut.ca",
}
var imageURLS = map[Country]string{
	USA:    "https://pizzahut.com/images/nationalMenu*",
	CANADA: "https://api.pizzahut.io/v1/content/en-ca/ca-1/images/",
}

type QuikOrder struct {
	country    Country
	apiURL     string
	imageURL   string
	response   []byte
	placeOrder bool
}

func (d *QuikOrder) SetResponse(data []byte) {
	d.response = data
}

func (d *QuikOrder) GetResponse() []byte {
	if d.response == nil {
		return []byte{0}
	}

	return d.response
}

func NewQuikOrder(r *http.Request) (*QuikOrder, error) {
	d := QuikOrder{placeOrder: false}
	countryCode := r.Header.Get("X-WiiCountryCode")

	if countryCode == "18" {
		d.country = CANADA
	} else if countryCode == "49" {
		d.country = USA
	} else {
		return nil, InvalidCountry
	}

	d.apiURL = apiURLS[d.country]
	d.imageURL = imageURLS[d.country]

	return &d, nil
}

