package telepathy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// ByteContent is a general type for message content that can be
// stored in byte array. ex: Image
type ByteContent struct {
	Type    string
	Content []byte
}

// Image manages all access ways for a image
type Image struct {
	ByteContent
	fullURL string
	logger  *logrus.Entry
}

// NewImage creates a Image with its binary representation
func NewImage(content ByteContent) *Image {
	ret := Image{
		ByteContent: content,
		logger:      logrus.WithField("module", "image")}
	return &ret
}

// FullURL returns the URL of the Image
// Image will be uploaded to CDN and generates a URL
func (img *Image) FullURL() (string, error) {
	if img.fullURL == "" {
		var err error
		img.fullURL, err = uploadImage(&img.ByteContent)
		if err != nil {
			return "", err
		}
		img.logger.Info("upload to imgur success: " + img.fullURL)
	}
	return img.fullURL, nil
}

// SmallThumbnailURL returns the URL of small thumbnail of the Image
func (img *Image) SmallThumbnailURL() (string, error) {
	url, err := img.FullURL()
	if err != nil {
		return "", err
	}
	// Works only for imgur
	dotIdx := strings.LastIndexByte(url, '.')
	return url[:dotIdx] + "l" + url[dotIdx:], nil
}

func uploadImage(content *ByteContent) (string, error) {
	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	imageType := strings.Split(content.Type, "/")[1]
	fileWriter, err := multipartWriter.CreateFormFile("image", "telepathy-uploaded."+imageType)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(fileWriter, bytes.NewReader(content.Content))
	if err != nil {
		return "", err
	}
	multipartWriter.Close()
	req, err := http.NewRequest("POST", "https://api.imgur.com/3/image", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("Authorization", "Client-ID "+os.Getenv("IMGUR_CLIENT_ID"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	data, _ := result["data"].(map[string]interface{})
	success, _ := result["success"].(bool)
	if !success {
		errStr, _ := data["error"].(string)
		return "", errors.New(errStr)
	}

	link, _ := data["link"].(string)

	return link, nil
}
