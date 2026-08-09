package cryptor

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

func NewAES(secret string) Cryptor {
	return &aesCrypto{
		Secret: secret,
	}
}

type aesCrypto struct {
	Secret string
}

// Encrypt 使用随机 IV（CBC 要求每次加密使用不同的 IV），并把 IV 拼在密文前面一起返回，
func (a *aesCrypto) Encrypt(text string) (string, error) {
	block, err := aes.NewCipher([]byte(a.Secret))
	if err != nil {
		return "", err
	}

	plainBytes := pkcs7Pad([]byte(text), aes.BlockSize)

	// 随机 IV，每次加密都不同
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	// 密文 = IV || AES-CBC(明文)
	cipherText := make([]byte, aes.BlockSize+len(plainBytes))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(cipherText[aes.BlockSize:], plainBytes)
	copy(cipherText[:aes.BlockSize], iv)

	return base64.StdEncoding.EncodeToString(cipherText), nil
}

// Decrypt 解密 Encrypt 生成的密文（密文 = IV||AES-CBC(明文)）。
// 必须校验 base64 解码结果与长度，避免非法输入导致 panic。
func (a *aesCrypto) Decrypt(text string) (string, error) {
	block, err := aes.NewCipher([]byte(a.Secret))
	if err != nil {
		return "", err
	}

	cipherBytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}

	// 新格式：至少包含 IV(16) + 一个分组，且总长度为分组整数倍
	if len(cipherBytes) < aes.BlockSize*2 || len(cipherBytes)%aes.BlockSize != 0 {
		return "", errors.New("invalid ciphertext")
	}
	iv := cipherBytes[:aes.BlockSize]
	ct := cipherBytes[aes.BlockSize:]
	return cbcDecrypt(block, iv, ct)
}

func cbcDecrypt(block cipher.Block, iv, cipherText []byte) (string, error) {
	if len(cipherText) == 0 || len(cipherText)%aes.BlockSize != 0 {
		return "", errors.New("invalid ciphertext length")
	}
	plainBytes := make([]byte, len(cipherText))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plainBytes, cipherText)

	plainBytes, err := pkcs7Unpad(plainBytes)
	if err != nil {
		return "", err
	}
	return string(plainBytes), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padBytes := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padBytes...)
}

// pkcs7Unpad 校验填充合法性，非法时返回 error 而非 panic。
func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 || length%aes.BlockSize != 0 {
		return nil, errors.New("invalid padding")
	}
	unpadding := int(data[length-1])
	if unpadding == 0 || unpadding > aes.BlockSize || unpadding > length {
		return nil, errors.New("invalid padding")
	}
	// 校验所有填充字节是否一致，抵御填充预言攻击
	for _, b := range data[length-unpadding:] {
		if int(b) != unpadding {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:length-unpadding], nil
}
