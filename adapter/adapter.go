package adapter

import (
	"bitbucket.org/oov/dgf"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type LevelDBAdapter struct {
	db *leveldb.DB
}

func NewLevelDBAdapter(db *leveldb.DB) *LevelDBAdapter {
	return &LevelDBAdapter{
		db: db,
	}
}

func (l *LevelDBAdapter) Close() error {
	l.db.Close()
	return nil
}

func (l *LevelDBAdapter) Delete(key []byte) error {
	return l.db.Delete(key, &opt.WriteOptions{})
}

func (l *LevelDBAdapter) Get(key []byte) ([]byte, error) {
	r, err := l.db.Get(key, &opt.ReadOptions{})
	if err != nil && err.Error() == "not found" {
		err = dgf.ErrKVSKeyNotFound
	}
	return r, err
}

func (l *LevelDBAdapter) Set(key, value []byte) error {
	return l.db.Put(key, value, &opt.WriteOptions{})
}
