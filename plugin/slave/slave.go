package slave

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/hexhub-agent-plugin/plugin"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

var mThisEndpoint string

func handleInterceptor(h http.HandlerFunc) http.HandlerFunc {
	return func(write http.ResponseWriter, req *http.Request) {
		token := req.Header.Get("Token")
		if token != plugin.Token {
			write.WriteHeader(500)
			return
		}
		h(write, req)
	}
}

func Start() {
	manifest, err := plugin.GetMyManifest()
	if err != nil {
		panic(err)
	}
	debug := flag.Bool("debug", true, "")
	token := flag.String("token", "", "")
	namespace := flag.String("namespace", "hexhub-dev", "")
	apiEndpoint := flag.String("apiEndpoint", "http://localhost:8080", "")
	masterPort := flag.Int("masterPort", 35580, "")
	flag.Parse()

	plugin.Init(manifest.PluginId, *namespace, *apiEndpoint, *masterPort, *debug, *token)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	logger.Info(listener.Addr().String())
	mThisEndpoint = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	RegisterRoute("/kill", func(writer http.ResponseWriter, request *http.Request) {
		os.Exit(0)
	})
	RegisterRoute("/ping", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		_, err := writer.Write([]byte("ok"))
		if err != nil {
			logger.Error(err)
		}
	})

	registerPlugin(manifest)
	heartbeat()

	panic(http.Serve(listener, nil))
}

func RegisterRoute(pattern string, f func(http.ResponseWriter, *http.Request)) {
	http.HandleFunc(pattern, handleInterceptor(f))
}

func Post(pluginId string, uri string, req any, result any) error {
	uri = strings.TrimLeft(uri, "/")
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
	//生成要访问的url
	var url string
	if pluginId == "" {
		url = fmt.Sprintf("%s/%s", plugin.AgentEndpoint, uri)
	} else {
		url = fmt.Sprintf("%s/%s/%s", plugin.AgentEndpoint, pluginId, uri)
	}

	//提交请求
	request, err := http.NewRequest("POST", url, reqBuf)

	//增加header选项
	request.Header.Add("Token", plugin.Token)
	request.Header.Add("PluginId", plugin.CurrentPluginId)
	request.Header.Add("Accept", "application/json")
	request.Header.Add("Content-Type", "application/json")

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

func GetMyEndpoint() string {
	return mThisEndpoint
}

func registerPlugin(manifest plugin.Manifest) {
	manifest.Endpoint = GetMyEndpoint()
	err := Post("", "/plugin/register", manifest, nil)
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

// heartbeat 定时检查主程是否退出
func heartbeat() {
	go func() {
		for true {
			time.Sleep(time.Second * 10)
			err := Post("", "ping", nil, nil)
			if err != nil {
				panic(err)
			}
		}
	}()
}
