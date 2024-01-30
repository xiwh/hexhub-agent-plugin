package util

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"github.com/akamensky/base58"
	"io"
	"math/rand"
)

func RandKey(size int) []byte {
	b := make([]byte, size, size)
	for i := 0; i < size; i++ {
		b[i] = byte(rand.Int() % 256)
	}
	return b
}

// AesEncryptJson AES json对象加密并返回base58
func AesEncryptJson(v any, key []byte) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return AesEncryptString(string(b), key)
}

// AesDecryptJson AES 解密base58并返回json序列化对象
func AesDecryptJson(base58Val string, v any, key []byte) error {
	b, err := AesDecryptString(base58Val, key)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(b), v)
	if err != nil {
		return err
	}
	return nil
}

func compressBytes(input []byte) ([]byte, error) {
	var compressedBuffer bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&compressedBuffer, gzip.BestCompression)

	_, err := writer.Write(input)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return compressedBuffer.Bytes(), nil
}

func decompressBytes(input []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decompressedData, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return decompressedData, nil
}

// AesEncryptString AES字符串加密并返回base58
func AesEncryptString(data string, key []byte) (string, error) {
	b, err := compressBytes([]byte(data))
	if err != nil {
		return "", err
	}

	b, err = AesEncrypt(b, key)
	if err != nil {
		return "", err
	}

	return base58.Encode(b), nil
}

// AesDecryptString AES字符串解密base58并返回解密字符串
func AesDecryptString(base58Val string, key []byte) (string, error) {
	b, err := base58.Decode(base58Val)
	if err != nil {
		return "", err
	}
	b, err = AesDecrypt(b, key)
	if err != nil {
		return "", err
	}
	b, err = decompressBytes(b)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AesEncrypt AES加密
func AesEncrypt(data []byte, key []byte) ([]byte, error) {
	// 分组秘钥
	// NewCipher该函数限制了输入k的长度必须为16, 24或者32
	block, _ := aes.NewCipher(key)
	// 获取秘钥块的长度
	blockSize := block.BlockSize()
	// 补全码
	data = PKCS7Padding(data, blockSize)
	// 加密模式
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	// 创建数组
	encrypted := make([]byte, len(data))
	// 加密
	blockMode.CryptBlocks(encrypted, data)

	//压缩
	buf := bytes.NewBuffer(nil)
	// Create destination writer
	gzipBuf, err := gzip.NewWriterLevel(buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	_, err = gzipBuf.Write(encrypted)
	if err != nil {
		return nil, err
	}
	err = gzipBuf.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func AesDecrypt(data []byte, key []byte) ([]byte, error) {
	//解压缩
	reader := bytes.NewReader(data)
	gzipWriter, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	_, err = io.Copy(buf, gzipWriter)
	if err != nil {
		return nil, err
	}
	_ = gzipWriter.Close()
	data = buf.Bytes()

	// 分组秘钥
	block, _ := aes.NewCipher(key)
	// 获取秘钥块的长度
	blockSize := block.BlockSize()
	// 加密模式
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	// 创建数组
	orig := make([]byte, len(data))
	// 解密
	blockMode.CryptBlocks(orig, data)
	// 去补全码
	orig = PKCS7UnPadding(orig)

	return orig, nil
}

// PKCS7Padding 补码 AES加密数据块分组长度必须为128bit(byte[16])，密钥长度可以是128bit(byte[16])、192bit(byte[24])、256bit(byte[32])中的任意一个。
func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padText...)
}

// PKCS7UnPadding 去码
func PKCS7UnPadding(origData []byte) []byte {
	length := len(origData)
	unPadding := int(origData[length-1])
	return origData[:(length - unPadding)]
}
