package cryptor

type Encryptor interface {
	Encrypt(string) (string, error)
}

type Decryptor interface {
	Decrypt(string) (string, error)
}

type Cryptor interface {
	Encryptor
	Decryptor
}
