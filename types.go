package hangups

// UploadFile contains information of image before and after image Upload
type UploadFile struct {
	name      string // file name
	size      int    // file size
	data      []byte // image data from file or base64 encoded string
	uploadURL string // actual url for image upload
}

// Photo contains information of image upload for message sending
type Photo struct {
	ImageID string `json:"photoid"`
	URL     string `json:"url"`
}
