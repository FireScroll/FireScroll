package main

import (
	"errors"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"golang.org/x/sync/errgroup"
	"log"
	"time"
)

func main() {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		panic(err)
	}

	db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("hey"), []byte("ho"))
	})

	g := errgroup.Group{}

	g.Go(func() error {
		return db.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte("hey"))
			time.Sleep(time.Second)
			return err
		})
	})
	g.Go(func() error {
		time.Sleep(time.Millisecond * 500)
		return db.Update(func(txn *badger.Txn) error {
			item, _ := txn.Get([]byte("hey"))
			item.Value(func(val []byte) error {
				fmt.Println(string(val))
				return nil
			})
			return txn.Set([]byte("hey"), []byte("a new val"))
		})
	})

	err = g.Wait()
	if err != nil {
		log.Fatalln("error with update during read", err)
	}

	db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("hey"))
		item.Value(func(val []byte) error {
			fmt.Println(string(val))
			return nil
		})
		return err
	})

	g.Go(func() error {
		time.Sleep(time.Millisecond * 500)
		return db.View(func(txn *badger.Txn) error {
			_, err := txn.Get([]byte("hey"))
			return err
		})
	})
	g.Go(func() error {
		return db.Update(func(txn *badger.Txn) error {
			item, _ := txn.Get([]byte("hey"))
			item.Value(func(val []byte) error {
				fmt.Println(string(val))
				return nil
			})
			time.Sleep(time.Second)
			return txn.Set([]byte("hey"), []byte("a new val 1"))
		})
	})

	err = g.Wait()
	if err != nil {
		log.Fatalln("error with read during update", err)
	}

	// THIS SECTION
	g.Go(func() error {
		err := db.Update(func(txn *badger.Txn) error {
			item, err := txn.Get([]byte("hey"))
			item.Value(func(val []byte) error {
				fmt.Println(string(val))
				return nil
			})
			time.Sleep(time.Millisecond * 500)
			err = txn.Set([]byte("hey"), []byte("a new val 3"))
			return err
		})
		return err
	})
	g.Go(func() error {
		time.Sleep(time.Millisecond * 250)
		err := db.Update(func(txn *badger.Txn) error {
			err := txn.Set([]byte("hey"), []byte("a new val 2"))
			return err
		})
		return err
	})
	err = g.Wait()
	if err != nil {
		fmt.Println(errors.Is(err, badger.ErrConflict))
		log.Println("This error should have happened, it's a sanity check", err)
	}

	db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("hey"))
		item.Value(func(val []byte) error {
			fmt.Println(string(val))
			return nil
		})
		return err
	})
}
