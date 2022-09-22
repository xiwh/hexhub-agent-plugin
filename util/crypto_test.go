package util

import "testing"

func TestAes(t *testing.T) {
	key := RandKey(24)
	v := AesEncryptString("aaaaaaaaabbbbbbbbbbb", key)
	println(v)
	v2, err := AesDecryptString(v, key)
	println(v2, err)
	if err != nil {
		t.Fatal(err)
	}
	if v2 != "aaaaaaaaabbbbbbbbbbb" {
		t.Fail()
	}
}
