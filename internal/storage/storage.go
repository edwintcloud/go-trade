package storage

type StorageType int

const (
	StorageTypeFile StorageType = iota
	StorageTypeMemory
)

type Storage interface {
	Save(storageType StorageType, key string, value []byte) error
	Load(key string) ([]byte, error)
	Delete(key string) error
}
