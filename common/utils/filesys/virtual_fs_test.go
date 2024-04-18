package filesys

import (
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
)

func Test_Virtual_FS_AddFile(t *testing.T) {
	t.Run("simple add file", func(t *testing.T) {
		vs := NewVirtualFs()
		vs.AddFileByString("main.go", "package main")

		file, err := vs.Open("main.go")
		require.NoError(t, err)
		_, err = file.Stat()
		require.NoError(t, err)
		data, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, []byte("package main"), data)
	})

	t.Run("simple read from file", func(t *testing.T) {
		vs := NewVirtualFs()
		vs.AddFileByString("main.go", "package main")

		file, err := vs.Open("main.go")
		require.NoError(t, err)

		data := make([]byte, 2)

		n, err := file.Read(data)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []byte("pa"), data)

		n, err = file.Read(data)
		require.NoError(t, err)
		require.Equal(t, 2, n)
		require.Equal(t, []byte("ck"), data)
	})

	t.Run("overwrite file", func(t *testing.T) {
		vs := NewVirtualFs()
		vs.AddFileByString("Main.go", "test")
		vs.AddFileByString("Main.go", "package main")
		file, err := vs.Open("Main.go")
		if err != nil {
			t.Fatalf("vs.Open want to get a file,but got [%v]", err)
		}
		fileInfo, err := file.Stat()
		if err != nil {
			t.Fatalf("file.Stat want to get fileinfo,but got [%v]", err)
		}
		fmt.Printf("virtualFs's  file info:%v\n", fileInfo)
	})

}

func Test_Virtual_FS_RemoveFile(t *testing.T) {
	vs := NewVirtualFs()
	vf := NewVirtualFile("Main.java", "Class Main(){}")

	vs.AddFile(vf)
	vs.RemoveFile("Main.java")
	file, _ := vs.Open("Main.java")
	if file != nil {
		fileInfo, _ := file.Stat()
		t.Fatalf("Open Main.java want to get [nil],but got [%v]", fileInfo)
	}
}

func Test_Virtual_Fs_Dir(t *testing.T) {
	t.Run("add virtual dir", func(t *testing.T) {
		vs := NewVirtualFs()
		vs.AddFileByString("Main.java", "package main;\nClass Main(){}")

		vs.AddDirByString("com")
		vs.AddFileToDir("com", "test.java", "package com.test")

		fileInfos, err := vs.ReadDir("com")
		if err != nil {
			t.Fatalf("want to get fileInfos,but got [%v]", err)
		}

		for _, fileInfo := range fileInfos {
			fmt.Printf("fileInfo:%v\n", fileInfo)
			if fileInfo.Name() != "test.java" {
				t.Fatalf("want to get fileInfo [Test.java],but got [%v]", fileInfo.Name())
			}
		}
	})

	t.Run("remove virtual dir", func(t *testing.T) {
		vs := NewVirtualFs()
		vs.AddFileByString("Main.java", "package main;\nClass Main(){}")

		vs.AddDirByString("com")
		vs.AddFileToDir("com", "Test.java", "package com.test")

		err := vs.RemoveDir("com")
		if err != nil {
			t.Fatalf("want to remove dir,but got [%v]", err)
		}

		fileInfos, _ := vs.ReadDir("com")
		if fileInfos != nil {
			t.Fatalf("fileInfos want [nil],but got [%v]", fileInfos)
		}

	})

}

func Test_virtual_fs(t *testing.T) {
	/*
		project:
			1.txt  "1"
			a:
				2.txt "2"
			b:
				3.txt "3"
	*/
	vs := NewVirtualFs()
	vs.AddDirByString("project")
	vs.AddFileToDir("project", "1.txt", "1")

	vs.AddDirByString("project", "a")
	vs.AddFileToDir("project/a", "2.txt", "2")

	vs.AddDirByString("project", "b")
	vs.AddFileToDir("project/b", "3.txt", "3")

	dir := make([]string, 0, 3)
	file := make([]string, 0, 3)

	err := Recursive("project",
		WithFileSystem(vs),
		WithDirStat(func(s string, fi fs.FileInfo) error {
			log.Infof("dir: %s", s)
			dir = append(dir, s)
			return nil
		}),
		WithFileStat(func(s string, f fs.File, fi fs.FileInfo) error {
			log.Infof("file: %s", s)
			file = append(file, s)
			return nil
		}),
	)
	require.NoError(t, err, err)
	require.Equal(t, []string{"project/a", "project/b"}, dir)
	require.Equal(t, []string{"project/1.txt", "project/a/2.txt", "project/b/3.txt"}, file)
}
