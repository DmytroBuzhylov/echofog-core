package storage

import (
	"time"

	"github.com/dgraph-io/badger/v4"
	jsoniter "github.com/json-iterator/go"
)

type BadgerStorage struct {
	db *badger.DB
}

func NewBadgerStorage(db *badger.DB) *BadgerStorage {
	bs := &BadgerStorage{
		db: db,
	}
	bs.GCWorker()
	return bs
}

func (s *BadgerStorage) Update(key, value []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

func (s *BadgerStorage) UpdateTTL(key, value []byte, ttl time.Duration) error {
	return s.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(ttl)

		return txn.SetEntry(e)
	})
}

func (s *BadgerStorage) Read(key []byte) ([]byte, error) {
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			data = make([]byte, len(val))
			copy(data, val)
			return nil
		})
	})
	return data, err
}

func (s *BadgerStorage) Delete(key []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (s *BadgerStorage) GCWorker() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
		again:
			err := s.db.RunValueLogGC(0.5)
			if err == nil {
				goto again
			}
		}
	}()
}

func (s *BadgerStorage) Set(key, value []byte) error {
	return s.Update(key, value)
}

func (s *BadgerStorage) Get(key []byte) ([]byte, error) {
	return s.Read(key)
}

func (s *BadgerStorage) FindValues(prefix []byte) ([]interface{}, error) {
	var values []interface{}
	err := s.db.View(func(txn *badger.Txn) error {

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			err := item.Value(func(v []byte) error {
				var val interface{}
				if err := jsoniter.Unmarshal(v, &val); err != nil {
					return err
				}
				values = append(values, val)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	return values, err
}

func (s *BadgerStorage) Exists(key []byte) bool {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false
	}
	return true
}
