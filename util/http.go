package util

import (
	"fmt"
	uuid "github.com/satori/go.uuid"
	"io"
	"net/http"
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
