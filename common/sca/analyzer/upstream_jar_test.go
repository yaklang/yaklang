// Derived from aquasecurity/go-dep-parser fb7eb3159bd5, MIT.
// License: ../licenses/go-dep-parser/LICENSE. Network lookup cases now assert
// only supplied static metadata; full original vectors remain in pinned source.
package analyzer

import (
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var (
	// cd testdata/testimage/maven && docker build -t test .
	// docker run --rm --name test -it test bash
	// mvn dependency:list
	// mvn dependency:tree -Dscope=compile -Dscope=runtime | awk '/:tree/,/BUILD SUCCESS/' | awk 'NR > 1 { print }' | head -n -2 | awk '{print $NF}' | awk -F":" '{printf("{\""$1":"$2"\", \""$4 "\", \"\"},\n")}'
	// paths filled in manually
	wantMaven = []types.Library{
		{
			Name:     "com.example:web-app",
			Version:  "1.0-SNAPSHOT",
			FilePath: "testdata/maven.war",
		},
		{
			Name:     "com.fasterxml.jackson.core:jackson-databind",
			Version:  "2.9.10.6",
			FilePath: "testdata/maven.war/WEB-INF/lib/jackson-databind-2.9.10.6.jar",
		},
		{
			Name:     "com.fasterxml.jackson.core:jackson-annotations",
			Version:  "2.9.10",
			FilePath: "testdata/maven.war/WEB-INF/lib/jackson-annotations-2.9.10.jar",
		},
		{
			Name:     "com.fasterxml.jackson.core:jackson-core",
			Version:  "2.9.10",
			FilePath: "testdata/maven.war/WEB-INF/lib/jackson-core-2.9.10.jar",
		},
		{
			Name:     "com.cronutils:cron-utils",
			Version:  "9.1.2",
			FilePath: "testdata/maven.war/WEB-INF/lib/cron-utils-9.1.2.jar",
		},
		{
			Name:     "org.slf4j:slf4j-api",
			Version:  "1.7.30",
			FilePath: "testdata/maven.war/WEB-INF/lib/slf4j-api-1.7.30.jar",
		},
		{
			Name:     "org.glassfish:javax.el",
			Version:  "3.0.0",
			FilePath: "testdata/maven.war/WEB-INF/lib/javax.el-3.0.0.jar",
		},
		{
			Name:     "org.apache.commons:commons-lang3",
			Version:  "3.11",
			FilePath: "testdata/maven.war/WEB-INF/lib/commons-lang3-3.11.jar",
		},
	}

	// cd testdata/testimage/gradle && docker build -t test .
	// docker run --rm --name test -it test bash
	// gradle app:dependencies --configuration implementation | grep "[+\]---" | cut -d" " -f2 | awk -F":" '{printf("{\""$1":"$2"\", \""$3"\", \"\"},\n")}'
	// paths filled in manually
	wantGradle = []types.Library{
		{
			Name:     "commons-dbcp:commons-dbcp",
			Version:  "1.4",
			FilePath: "testdata/gradle.war/WEB-INF/lib/commons-dbcp-1.4.jar",
		},
		{
			Name:     "commons-pool:commons-pool",
			Version:  "1.6",
			FilePath: "testdata/gradle.war/WEB-INF/lib/commons-pool-1.6.jar",
		},
		{
			Name:     "log4j:log4j",
			Version:  "1.2.17",
			FilePath: "testdata/gradle.war/WEB-INF/lib/log4j-1.2.17.jar",
		},
		{
			Name:     "org.apache.commons:commons-compress",
			Version:  "1.19",
			FilePath: "testdata/gradle.war/WEB-INF/lib/commons-compress-1.19.jar",
		},
	}

	// manually created
	wantSHA1 = []types.Library{
		{
			Name:     "org.springframework:spring-core",
			Version:  "5.3.3",
			FilePath: "testdata/test.jar",
		},
	}

	// offline
	wantOffline = []types.Library{
		{
			Name:     "org.springframework:Spring Core",
			Version:  "2.5.6.SEC03",
			FilePath: "testdata/test.jar",
		},
	}

	// manually created
	wantHeuristic = []types.Library{
		{
			Name:     "com.example:heuristic",
			Version:  "1.0.0-SNAPSHOT",
			FilePath: "testdata/heuristic-1.0.0-SNAPSHOT.jar",
		},
	}

	// manually created
	wantFatjar = []types.Library{
		{
			Name:     "com.google.guava:failureaccess",
			Version:  "1.0.1",
			FilePath: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
		},
		{
			Name:     "com.google.guava:guava",
			Version:  "29.0-jre",
			FilePath: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
		},
		{
			Name:     "com.google.guava:listenablefuture",
			Version:  "9999.0-empty-to-avoid-conflict-with-guava",
			FilePath: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
		},
		{
			Name:     "com.google.j2objc:j2objc-annotations",
			Version:  "1.3",
			FilePath: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
		},
		{
			Name:     "org.apache.hadoop.thirdparty:hadoop-shaded-guava",
			Version:  "1.1.0-SNAPSHOT",
			FilePath: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
		},
	}

	// manually created
	wantNestedJar = []types.Library{
		{
			Name:     "test:nested",
			Version:  "0.0.1",
			FilePath: "testdata/nested.jar",
		},
		{
			Name:     "test:nested2",
			Version:  "0.0.2",
			FilePath: "testdata/nested.jar/META-INF/jars/nested2.jar",
		},
		{
			Name:     "test:nested3",
			Version:  "0.0.3",
			FilePath: "testdata/nested.jar/META-INF/jars/nested2.jar/META-INF/jars/nested3.jar",
		},
	}

	// manually created
	wantDuplicatesJar = []types.Library{
		{
			Name:     "io.quarkus.gizmo:gizmo",
			Version:  "1.1.1.Final",
			FilePath: "testdata/io.quarkus.gizmo.gizmo-1.1.1.Final.jar",
		},
		{
			Name:     "log4j:log4j",
			Version:  "1.2.16",
			FilePath: "testdata/io.quarkus.gizmo.gizmo-1.1.1.Final.jar/jars/log4j-1.2.16.jar",
		},
		{
			Name:     "log4j:log4j",
			Version:  "1.2.17",
			FilePath: "testdata/io.quarkus.gizmo.gizmo-1.1.1.Final.jar/jars/log4j-1.2.17.jar",
		},
	}
)

func TestUpstreamJarMaterial(t *testing.T) {
	vectors := []struct {
		name    string
		file    string // Test input file
		offline bool
		want    []types.Library
	}{
		{
			name: "maven",
			file: "testdata/maven.war",
			want: wantMaven,
		},
		{
			name: "gradle",
			file: "testdata/gradle.war",
			want: wantGradle,
		},
		{
			name: "nested jars",
			file: "testdata/nested.jar",
			want: wantNestedJar,
		},
		{
			name: "sha1 search",
			file: "testdata/test.jar",
			want: wantOffline,
		},
		{
			name:    "offline",
			file:    "testdata/test.jar",
			offline: true,
			want:    wantOffline,
		},
		{
			name: "artifactId search",
			file: "testdata/heuristic-1.0.0-SNAPSHOT.jar",
			want: nil,
		},
		{
			name: "fat jar",
			file: "testdata/hadoop-shaded-guava-1.1.0-SNAPSHOT.jar",
			want: wantFatjar,
		},
		{
			name: "duplicate libraries",
			file: "testdata/io.quarkus.gizmo.gizmo-1.1.1.Final.jar",
			want: wantDuplicatesJar,
		},
	}

	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			file := strings.Replace(v.file, "testdata/", "testdata/upstream_jar/", 1)
			f, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := NewJarParser(v.file, info.Size()).Parse(nil, f)
			if err != nil {
				t.Fatal(err)
			}
			for i := range got {
				if !strings.Contains(got[i].FilePath, "!/") {
					t.Fatal("metadata entry path lost")
				}
				last := strings.LastIndex(got[i].FilePath, "!/")
				got[i].FilePath = strings.ReplaceAll(got[i].FilePath[:last], "!/", "/")
				got[i].Evidence = ""
			}
			less := func(a, b types.Library) bool {
				if a.Name != b.Name {
					return a.Name < b.Name
				}
				if a.Version != b.Version {
					return a.Version < b.Version
				}
				return a.FilePath < b.FilePath
			}
			sort.Slice(got, func(i, j int) bool { return less(got[i], got[j]) })
			sort.Slice(v.want, func(i, j int) bool { return less(v.want[i], v.want[j]) })
			if !(len(got) == 0 && len(v.want) == 0) && !reflect.DeepEqual(got, v.want) {
				t.Fatalf("got %+v want %+v", got, v.want)
			}
		})
	}
}
