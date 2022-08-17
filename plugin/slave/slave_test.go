package slave

import (
	"net/http"
	"testing"
)

func Test(t *testing.T) {
	SetToken("92de28a6-53a2-4500-96c1-642b5ff4b85f")
	http.HandleFunc("test", func(writer http.ResponseWriter, request *http.Request) {
		println("aaaaa")
	})
	Start("abc")
}
