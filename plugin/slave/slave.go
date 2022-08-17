package slave

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"gaydev-agent-plugin/plugin"
	"github.com/wonderivan/logger"
	"io"
	"net"
	"net/http"
	"os"
)

var masterEndpoint string
var mToken string
var mThisEndpoint string
var mPluginId string

type pluginHandler struct {
}

func (t *pluginHandler) ServeHTTP(write http.ResponseWriter, req *http.Request) {
	token := req.Header.Get("Token")
	if token != mToken {
		write.WriteHeader(500)
	}
}

func SetToken(token string) {
	mToken = token
}

func Start(pluginId string) {
	plugin.Init()
	mPluginId = pluginId
	masterEndpoint = *flag.String("address", plugin.AgentEndpoint, "")
	if mToken == "" {
		mToken = *flag.String("token", "", "")
		if mToken == "" {
			panic("token is empty")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	println(listener.Addr())
	mThisEndpoint = fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	registerPlugin()
	http.HandleFunc("kill", func(writer http.ResponseWriter, request *http.Request) {
		os.Exit(0)
	})
	err = http.Serve(listener, new(pluginHandler))
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

func registerPlugin() {
	manifest, err := plugin.GetMyManifest()
	manifest.Endpoint = GetMyEndpoint()
	if err != nil {
		panic(err)
	}
	err = Post("register-plugin", manifest, nil)
	if err != nil {
		logger.Error(err)
		panic(err)
	}
}

func Post(uri string, req any, result any) error {
	var reqBuf *bytes.Buffer
	if req != nil {
		reqData, err := json.Marshal(req)
		if err != nil {
			return err
		}
		reqBuf = bytes.NewBuffer(reqData)
	}

	client := &http.Client{}
	//生成要访问的url
	url := fmt.Sprintf("%s/%s", masterEndpoint, uri)

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
	response, _ := client.Do(request)
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
