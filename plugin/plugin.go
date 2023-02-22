package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/util"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

var ApiEndpoint string
var AgentEndpoint string
var AgentAddr string
var Debug bool
var Namespace string
var Token string
var MasterPort int
var CurrentPluginId string

var HomeDir string
var PluginsDir string

var aesKey []byte

type APIResult struct {
	Status int
	Msg    string
	Data   any
}

type Manifest struct {
	PluginId    string `json:"pluginId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     int    `json:"version"`
	VersionName string `json:"versionName"`
	ExecEnter   string `json:"execEnter"`
	AutoExit    bool   `json:"autoExit"`
	Endpoint    string `json:"endpoint"`
}

func Init(currentPluginId string, namespace string, apiEndpoint string, masterPort int, debug bool, token string) {
	aesKey = util.RandKey(24)
	MasterPort = masterPort
	CurrentPluginId = currentPluginId
	AgentAddr = fmt.Sprintf("127.0.0.1:%d", masterPort)
	AgentEndpoint = fmt.Sprintf("http://%s", AgentAddr)
	ApiEndpoint = apiEndpoint
	Namespace = namespace
	Debug = debug
	if debug {
		token = "debug"
	}
	Token = token
	if token == "" {
		panic("token is empty")
	} else {
		logger.Info("token:%s", token)
	}

	current, err := user.Current()
	if err != nil {
		panic(err)
	}
	HomeDir = filepath.Join(current.HomeDir, "."+namespace)
	PluginsDir = filepath.Join(HomeDir, "plugins")
	if !util.IsDir(PluginsDir) {
		err := os.MkdirAll(PluginsDir, 0755)
		if err != nil {
			logger.Error(err)
			panic(err)
		}
	}
}

func GetManifest(pluginId string) (Manifest, error) {
	thisPluginDir := filepath.Join(PluginsDir, pluginId)
	manifestFile := strings.Join([]string{thisPluginDir, string(os.PathSeparator), "manifest.json"}, "")
	manifest, err := readManifestFile(manifestFile)
	if err != nil {
		return manifest, err
	}
	if manifest.PluginId != pluginId {
		return manifest, fmt.Errorf("the plugin id is not match, the expected value is %s, but the actual value is %s", pluginId, manifest.PluginId)
	}
	return manifest, nil
}

func GetMyManifest() (Manifest, error) {
	manifestFile := strings.Join([]string{GetCurrentPath(), string(os.PathSeparator), "manifest.json"}, "")
	manifest, err := readManifestFile(manifestFile)
	return manifest, err
}

func GetManifests() (map[string]Manifest, error) {
	plugins, err := os.ReadDir(PluginsDir)
	if err != nil {
		return nil, err
	}
	manifests := make(map[string]Manifest, len(plugins))
	for _, pluginPath := range plugins {
		if pluginPath.IsDir() {
			manifest, err := GetManifest(pluginPath.Name())
			if err != nil {
				logger.Error(err)
				continue
			}
			manifests[manifest.PluginId] = manifest
		}
	}
	return manifests, nil
}

func GetCurrentPath() string {
	if ex, err := os.Executable(); err == nil {
		return filepath.Dir(ex)
	}
	return "./"
}

func GetCurrentPersistentDir() string {
	dataPath := path.Join(HomeDir, "data", CurrentPluginId)
	if util.IsDir(dataPath) {
		return dataPath
	}
	_ = os.MkdirAll(dataPath, os.ModePerm)
	return dataPath
}

func GetAESKey() []byte {
	return aesKey
}

func ApiPost(uri string, req any, result any) error {
	var reqBuf *bytes.Buffer
	if req != nil {
		reqData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		reqBuf = bytes.NewBuffer(reqData)
	} else {
		reqBuf = bytes.NewBufferString("")
	}

	client := &http.Client{}
	reqUrl := FormatApiUrl(uri)
	//提交请求
	request, err := http.NewRequest("POST", reqUrl, reqBuf)
	if err != nil {
		return err
	}
	//处理返回结果
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}(response.Body)
	if result == nil {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, result)
	return err
}

func ApiGet(uri string, params map[string]string, v any) error {
	client := &http.Client{}
	reqUrl := FormatApiUrl(uri)
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	queryStr := values.Encode()
	if queryStr != "" {
		reqUrl = reqUrl + "?" + queryStr
	}
	//提交请求
	request, err := http.NewRequest("GET", reqUrl, nil)
	if err != nil {
		return err
	}
	request.Header.Add("Hexhub-Client", CurrentPluginId)
	//处理返回结果
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error(err)
		}
	}(response.Body)
	if v == nil {
		return nil
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	result := new(APIResult)
	result.Data = v

	err = json.Unmarshal(data, result)
	if err != nil {
		return err
	}
	if result.Status != 0 {
		return fmt.Errorf(result.Msg)
	}
	return err
}

func FormatPluginUrl(req *http.Request, uri string) string {
	var reqUrl = req.URL
	var proxyUrl = req.Header.Get("Proxy-Url")
	if proxyUrl != "" {
		reqUrl, _ = url.Parse(proxyUrl)
	} else {
		reqUrl, _ = url.Parse(req.URL.Scheme + "://" + req.Host + req.RequestURI)
	}
	var scheme = reqUrl.Scheme
	var host = reqUrl.Hostname()
	var port = reqUrl.Port()
	uri = strings.TrimPrefix(uri, "/")
	if scheme == "http" && (port == "" || port == "80") {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, host, CurrentPluginId, uri)
	} else if scheme == "https" && (port == "" || port == "443") {
		return fmt.Sprintf("%s://%s/%s/%s", scheme, host, CurrentPluginId, uri)
	} else {
		return fmt.Sprintf("%s://%s:%s/%s/%s", scheme, host, port, CurrentPluginId, uri)
	}
}

func FormatApiUrl(uri string) string {
	apiEndPoint := strings.TrimRight(ApiEndpoint, "/")
	uri = strings.TrimLeft(uri, "/")
	return apiEndPoint + "/" + uri
}

func readManifestFile(manifestFile string) (manifest Manifest, err error) {
	manifestJson, err := os.ReadFile(manifestFile)
	if err != nil {
		return manifest, err
	}
	err = json.Unmarshal(manifestJson, &manifest)
	if err != nil {
		return manifest, err
	}
	return manifest, nil
}
