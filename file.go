package hangups

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/asaskevich/govalidator"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func randomName() string {
	return time.Now().Format("2006-01-02")
}

func readBase64Image(v string) (*UploadFile, error) {
	data, err := base64.StdEncoding.DecodeString(v)
	if err == nil {
		buff := make([]byte, 512)
		// copy will ensure the copied size
		copy(buff, data)
		isImage, imageType := getImageType(buff)
		if isImage {
			return &UploadFile{
				name: fmt.Sprintf("%s.%s", randomName(), imageType),
				size: len(data),
				data: data,
			}, nil
		}
		return nil, errors.New("not an image file")
	}
	return nil, errors.New("not an base64 encoded file")
}

func readImageFile(v string) (*UploadFile, error) {

	data, err := ioutil.ReadFile(v)

	if err != nil {
		return nil, err
	}

	return &UploadFile{
		name: filepath.Base(v),
		size: len(data),
		data: data,
	}, nil
}

// getImageType try to detect MIME type of a buffer and guess the image type from MIME type
func getImageType(buffer []byte) (bool, string) {
	mimeType := strings.ToLower(http.DetectContentType(buffer))

	switch mimeType {
	case "image/x-icon":
		return true, "ico"
	case "image/bmp":
		return true, "bmp"
	case "image/png":
		return true, "png"
	case "image/jpeg":
		return true, "jpeg"
	case "image/webp":
		return true, "webp"
	case "image/gif":
		return true, "gif"
	default:
		return false, ""
	}
}

// readImage try to read image data from a base64 encoded string or file or download from a url
func readImage(v string) (*UploadFile, error) {
	if file, err := readBase64Image(v); err == nil {
		return file, nil
	}

	if file, err := downloadImage(v); err == nil {
		return file, nil
	}

	return readImageFile(v)
}

// downloadImage download image from a http/https url
func downloadImage(url string) (*UploadFile, error) {
	if !govalidator.IsURL(url) {
		return nil, fmt.Errorf("%s is not an valid url", url)
	}

	resp, err := http.Get(url)

	if err != nil {
		return nil, fmt.Errorf("error requesting image from %s : %v", url, err)
	}

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("error downloading image from %s : %v", url, err)
	}

	return &UploadFile{
		name: filepath.Base(url),
		size: len(body),
		data: body,
	}, nil
}
