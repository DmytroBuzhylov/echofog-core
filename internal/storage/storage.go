package storage

type Storage interface {
	Set(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Delete(key []byte) error
	FindValues(prefix []byte) ([]interface{}, error)
	Exists(key []byte) bool
}
