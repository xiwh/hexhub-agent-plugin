package httputil

import (
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/xiwh/hexhub-agent-plugin/plugin/slave"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
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

func FormatUrl(req *http.Request, uri string) string {
	var reqUrl = req.URL
	var proxyUrl = req.Header.Get("Proxy-Url")
	if proxyUrl != "" {
		reqUrl, _ = url.Parse(proxyUrl)
	} else {
		reqUrl, _ = url.Parse("http://" + req.Host + req.RequestURI)
	}
	var scheme = reqUrl.Scheme
	var host = reqUrl.Hostname()
	var port = reqUrl.Port()
	uri = strings.TrimPrefix(uri, "/")
	if scheme == "http" && port == "80" {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, host, slave.GetPluginId(), uri)
	} else if scheme == "https" && port == "443" {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, host, slave.GetPluginId(), uri)
	} else {
		return fmt.Sprintf("%s://%s:%s/%s/%s", scheme, host, port, slave.GetPluginId(), uri)
	}
}
