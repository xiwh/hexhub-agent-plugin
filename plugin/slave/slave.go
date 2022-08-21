package slave

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/xiwh/gaydev-agent-plugin/plugin"
	"io"
	"net"
	"net/http"
	"os"
)

var masterEndpoint string
var mToken string
var mThisEndpoint string
var mPluginId string

func handleInterceptor(h http.HandlerFunc) http.HandlerFunc {
	return func(write http.ResponseWriter, req *http.Request) {
		if !plugin.Debug {
			token := req.Header.Get("Token")
			if token != mToken {
				write.WriteHeader(500)
				return
			}
		}
		h(write, req)
	}
}

func Start(pluginId string) {
	plugin.Init()
	mPluginId = pluginId
	masterEndpoint = *flag.String("address", plugin.AgentEndpoint, "")
	if mToken == "" && !plugin.Debug {
		mToken = *flag.String("token", "", "")
		if mToken == "" {
			panic("token is empty")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	logger.Info(listener.Addr().String())
	mThisEndpoint = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	registerPlugin()
	RegisterRoute("/kill", func(writer http.ResponseWriter, request *http.Request) {
		os.Exit(0)
	})
	RegisterRoute("/ping", func(writer http.ResponseWriter, request *http.Request) {
		println("slave ping")
		writer.WriteHeader(200)
		_, err := writer.Write([]byte("ok"))
		if err != nil {
			logger.Error(err)
		}
	})
	err = http.Serve(listener, nil)
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

func RegisterRoute(pattern string, f func(http.ResponseWriter, *http.Request)) {
	http.HandleFunc(pattern, handleInterceptor(f))
}

func registerPlugin() {
	manifest, err := plugin.GetMyManifest()
	manifest.Endpoint = GetMyEndpoint()
	if err != nil {
		panic(err)
	}
	err = Post("", "register-plugin", manifest, nil)
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

func Post(pluginId string, uri string, req any, result any) error {
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
		url = fmt.Sprintf("%s/%s", masterEndpoint, uri)
	} else {
		url = fmt.Sprintf("%s/%s/%s", masterEndpoint, pluginId, uri)
	}

	//提交请求
	request, err := http.NewRequest("POST", url, reqBuf)

	//增加header选项
	request.Header.Add("Token", mToken)
	request.Header.Add("PluginId", mPluginId)
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

func GetMasterEndpoint() string {
	return masterEndpoint
}

func GetMyEndpoint() string {
	return mThisEndpoint
}
