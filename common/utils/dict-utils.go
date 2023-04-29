package utils

import (
	"bufio"
	"github.com/pkg/errors"
	"io"
	"os"
)

type BruteDictParser struct {
	UserDictFile, PassDictFile *os.File
	UserDict, PassDict         *bufio.Scanner
	offset                     int64
	shouldUpdateUser           bool
}

func NewBruteDictParser(userDict, passDict string) (*BruteDictParser, error) {
	userDictFile, err := os.Open(userDict)
	if err != nil {
		return nil, errors.Errorf("failed to open user dict: %s", err)
	}

	passDictFile, err := os.Open(passDict)
	if err != nil {
		return nil, errors.Errorf("failed to open pass dict: %s", err)
	}

	userScanner := bufio.NewScanner(userDictFile)
	userScanner.Split(bufio.ScanLines)

	passScanner := bufio.NewScanner(passDictFile)
	passScanner.Split(bufio.ScanLines)
	//
	//for userScanner.Scan() {
	//	user := userScanner.Text()
	//	for passScanner.Scan() {
	//		pass := passScanner.Text()
	//
	//		currentUserOffset, _ := userDictFile.Seek(0, 1)
	//		currentPassOffset, _ := userDictFile.Seek(0, 1)
	//		log.Infof("fetching user: %s pass: %s offset: %d:%d", user, pass, currentUserOffset, currentPassOffset)
	//	}
	//}
	return &BruteDictParser{
		UserDictFile:     userDictFile,
		PassDictFile:     passDictFile,
		UserDict:         userScanner,
		PassDict:         passScanner,
		offset:           0,
		shouldUpdateUser: true,
	}, nil
}

type UserPassPair struct {
	Username, Password     string
	UserOffset, PassOffset int64
}

func (b *BruteDictParser) Next() (*UserPassPair, error) {
	if b.shouldUpdateUser {
		if !b.UserDict.Scan() {
			return nil, io.EOF
		}
		b.shouldUpdateUser = false
	}

	user := b.UserDict.Text()
	if !b.PassDict.Scan() {
		_, err := b.PassDictFile.Seek(0, 0)
		if err != nil {
			return nil, errors.Errorf("seek 0,0 failed: %s", err)
		}

		b.PassDict = bufio.NewScanner(b.PassDictFile)
		b.shouldUpdateUser = true
		if !b.PassDict.Scan() {
			return nil, errors.New("empty pass dict")
		}
	}
	pass := b.PassDict.Text()

	uOffset, err := b.UserDictFile.Seek(0, 1)
	if err != nil {
		return nil, errors.Errorf("seek 0,1 failed: %s", err)
	}

	pOffset, err := b.PassDictFile.Seek(0, 1)
	if err != nil {
		return nil, errors.Errorf("seek 0,1 failed: %s", err)
	}

	return &UserPassPair{
		Username: user, Password: pass,
		UserOffset: uOffset,
		PassOffset: pOffset,
	}, nil
}
