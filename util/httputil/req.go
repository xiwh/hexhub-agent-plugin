package httputil

import (
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"io"
	"net/http"
	"net/url"
	"os"
)

type writeCounter struct {
	Total    int64
	Current  int64
	Callback func(total int64, current int64)
}

func (t *writeCounter) Write(p []byte) (int, error) {

	n := len(p)
	t.Current += int64(n)
	t.Callback(t.Total, t.Current)
	return n, nil
}

func DownloadFile(url string, Callback func(total int64, current int64)) (string, error) {
	tempFilePath := fmt.Sprintf("%s/%s.tmp", os.TempDir(), uuid.NewV4().String())
	out, err := os.Create(tempFilePath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	counter := &writeCounter{
		Total:    resp.ContentLength,
		Current:  0,
		Callback: Callback,
	}
	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return "", err
	}
	return tempFilePath, nil
}

func ReadJsonBody(r *http.Request, value any) error {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func GetPageId(r *http.Request) string {
	pageId := r.Header.Get("Hexhub-Page-Id")
	if pageId == "" {
		_ = r.ParseForm()
		pageId = r.Form.Get("hexhubPageId")
	}
	return pageId
}

func GetSchemeHost(url *url.URL) string {
	port := url.Port()
	if port == "" {
		return fmt.Sprintf("%s://%s", url.Scheme, url.Hostname())
	} else {
		return fmt.Sprintf("%s://%s:%s", url.Scheme, url.Hostname(), port)
	}
}
