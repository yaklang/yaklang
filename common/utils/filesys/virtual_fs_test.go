package filesys

import (
	"fmt"
	"testing"
)

func Test_Virtual_FS_AddFile(t *testing.T) {
	t.Run("simple add file", func(t *testing.T) {
		vs := NewVirtualFs("C:\\windows\\project")
		vf1 := NewVirtualFile("test.txt", "test")
		vf2 := NewVirtualFile("Main.go", "package main")

		vs.AddFile(vf1)
		vs.AddFile(vf2)

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

	t.Run("force add file", func(t *testing.T) {
		vs := NewVirtualFs("C:\\windows\\project")
		vf1 := NewVirtualFile("Main.go", "test")
		vf2 := NewVirtualFile("Main.go", "package main")

		vs.AddFile(vf1)
		vs.AddFileForce(vf2)

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
	vs := NewVirtualFs("C:\\windows\\project")
	vf := NewVirtualFile("Main.java", "Class Main(){}")

	vs.AddFile(vf)
	err := vs.RemoveFile("Main.java")
	if err != nil {
		t.Fatalf("vs.RemoveFile want to remove file,but got [%v]", err)
	}

	file, _ := vs.Open("Main.java")
	if file != nil {
		fileInfo, _ := file.Stat()
		t.Fatalf("Open Main.java want to get [nil],but got [%v]", fileInfo)
	}
}

func Test_Virtual_FS_GetContent(t *testing.T) {
	vs := NewVirtualFs("C:\\windows\\project")
	vf := NewVirtualFile("Main.java", "Class Main(){}")
	vs.AddFile(vf)

	content, err := vs.GetContent("Main.java")
	if err != nil {
		t.Fatalf("vs.GetContent want to get content,but got [%v]", err)
	}
	fmt.Printf("content:\n%v\n", content)
}

func Test_Virtual_Fs_Dir(t *testing.T) {
	t.Run("add virtual dir", func(t *testing.T) {
		vs := NewVirtualFs("C:\\windows\\project")
		vf := NewVirtualFile("Main.java", "package main;\nClass Main(){}")
		vs.AddFile(vf)

		dir := NewVirtualFs("C:\\windows\\project\\com")
		fileInDir := NewVirtualFile("Test.java", "package com.test\nClass Main(){}")
		dir.AddFile(fileInDir)

		vs.AddDir("com", dir)

		fileInfos, err := vs.ReadDir("com")
		if err != nil {
			t.Fatalf("want to get fileInfos,but got [%v]", err)
		}

		for _, fileInfo := range fileInfos {
			fmt.Printf("fileInfo:%v\n", fileInfo)
			if fileInfo.Name() != "Test.java" {
				t.Fatalf("want to get fileInfo [Test.java],but got [%v]", fileInfo.Name())
			}
		}
	})

	t.Run("force add virtual dir", func(t *testing.T) {
		vs := NewVirtualFs("C:\\windows\\project")
		vf := NewVirtualFile("Main.java", "package main;\nClass Main(){}")
		vs.AddFile(vf)

		dir1 := NewVirtualFs("C:\\windows\\project\\com")
		fileInDir := NewVirtualFile("Test.java", "package com.test\nClass Main(){}")
		dir1.AddFile(fileInDir)
		dir2 := NewVirtualFs("C:\\windows\\project\\com")
		dir2.AddFile(fileInDir)

		vs.AddDir("com", dir1)
		vs.AddDirForce("com", dir2)

		fileInfos, err := vs.ReadDir("com")
		if err != nil {
			t.Fatalf("want to get fileInfos,but got [%v]", err)
		}

		for _, fileInfo := range fileInfos {
			fmt.Printf("fileInfo:%v\n", fileInfo)
			if fileInfo.Name() != "Test.java" {
				t.Fatalf("want to get fileInfo [Test.java],but got [%v]", fileInfo.Name())
			}
		}
	})

	t.Run("remove virtual dir", func(t *testing.T) {
		vs := NewVirtualFs("C:\\windows\\project")
		vf := NewVirtualFile("Main.java", "package main;\nClass Main(){}")
		vs.AddFile(vf)

		dir := NewVirtualFs("C:\\windows\\project\\com")
		fileInDir := NewVirtualFile("Test.java", "package com.test\nClass Main(){}")
		dir.AddFile(fileInDir)

		vs.AddDir("com", dir)

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
