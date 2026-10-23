package filesys

import "testing"

func TestSourceMaterializerTimingSeparatesDiskAndMemory(t *testing.T) {
	for _, disk := range []bool{false, true} {
		name := "memory"
		if disk {
			name = "disk"
		}
		t.Run(name, func(t *testing.T) {
			base := NewVirtualFs()
			base.AddFile("A.java", "class A {}")
			var options []SourceMaterializerOption
			if disk {
				options = append(options, WithSourceMaterializerDisk(t.TempDir(), "timing", 1<<20))
			}
			m := NewSourceMaterializer(options...)
			raw, err := m.ReadFile(base, "A.java")
			if err != nil {
				t.Fatal(err)
			}
			stats := m.Stats()
			if stats.BaseReadSeconds <= 0 || stats.StoreSeconds <= 0 {
				t.Fatalf("no source/read timing: %+v", stats)
			}
			if disk {
				if stats.DiskWriteSeconds <= 0 || stats.DiskWriteBytes != int64(len(raw)) {
					t.Fatalf("disk timing: %+v", stats)
				}
			} else if stats.DiskWriteSeconds != 0 || stats.DiskWriteBytes != 0 {
				t.Fatalf("memory mode wrote disk: %+v", stats)
			}
			if _, err = m.ReadFile(base, "A.java"); err != nil {
				t.Fatal(err)
			}
			again := m.Stats()
			if again.BaseReadSeconds != stats.BaseReadSeconds || again.DiskWriteSeconds != stats.DiskWriteSeconds {
				t.Fatalf("cache hit repeated source/disk work: before=%+v after=%+v", stats, again)
			}
		})
	}
}
