package util

import (
	"encoding/base64"
	"testing"
)

func TestAes(t *testing.T) {
	randStr := base64.StdEncoding.EncodeToString(RandKey(128))
	key := RandKey(24)
	v, err := AesEncryptString(randStr, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(v)
	v2, err := AesDecryptString(v, key)
	println(v2, err)
	if err != nil {
		t.Fatal(err)
	}
	if v2 != randStr {
		t.Fail()
	}
}
