package hangups

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

const apiKey = "AIzaSyAfFJCeph-euFSwtmqFZi0kaKk-cZ5wufM"

// GetAuthHeaders returns authentication header
func GetAuthHeaders(sapisid string) map[string]string {
	originURL := "https://talkgadget.google.com"
	timestampMsec := time.Now().Unix() * 1000

	authString := fmt.Sprintf("%d %s %s", timestampMsec, sapisid, originURL)
	hash := sha1.New()
	hash.Write([]byte(authString))
	hashBytes := hash.Sum(nil)
	hexSha1 := hex.EncodeToString(hashBytes)
	sapisidHash := fmt.Sprintf("SAPISIDHASH %d_%s", timestampMsec, hexSha1)
	return map[string]string{"Authorization": sapisidHash, "X-Origin": originURL, "X-Goog-Authuser": "0"}
}

// APIRequest performs an API Request
func (c *Client) APIRequest(endpointURL, responseType string, headers map[string]string, payload []byte) ([]byte, error) {

	authHeaders := GetAuthHeaders(c.Session.Sapisid)

	for headerKey, headerVal := range authHeaders {
		headers[headerKey] = headerVal
	}

	headers["User-Agent"] = c.UserAgent
	headers["Cookie"] = c.Session.Cookies
	// This header is required for Protocol Buffer responses, which causes
	// them to be base64 encoded:
	headers["X-Goog-Encode-Response-If-Executable"] = "base64"

	uri, err := url.Parse(endpointURL)
	if err != nil {
		return nil, err
	}
	urlParams := uri.Query()
	urlParams.Set("alt", responseType)
	urlParams.Set("key", apiKey)
	uri.RawQuery = urlParams.Encode()

	req, err := http.NewRequest(http.MethodPost, uri.String(), bytes.NewReader(payload))
	for headerKey, headerVal := range headers {
		req.Header.Set(headerKey, headerVal)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	return bodyBytes, nil
}
