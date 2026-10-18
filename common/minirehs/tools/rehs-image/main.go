// Command rehs-image compiles an ordered JSON []minirehs.Pattern into an image.
// Use it from go:generate; -check verifies an embedded image without rewriting it.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yaklang/yaklang/common/minirehs"
)

func main() {
	patterns := flag.String("patterns", "", "ordered JSON pattern file")
	out := flag.String("out", "", "output .rehs file")
	loc := flag.Bool("location", false, "report match locations")
	minLit := flag.Int("min-literal", 2, "minimum required literal length")
	check := flag.Bool("check", false, "check the committed image instead of writing")
	flag.Parse()
	if *patterns == "" || *out == "" {
		log.Fatal("-patterns and -out are required")
	}
	raw, err := os.ReadFile(*patterns)
	if err != nil {
		log.Fatal(err)
	}
	var pats []minirehs.Pattern
	if err = json.Unmarshal(raw, &pats); err != nil {
		log.Fatal(err)
	}
	db, err := minirehs.Compile(pats, minirehs.WithBackend(minirehs.BackendMVS), minirehs.WithReportLocation(*loc), minirehs.WithMinLiteralLen(*minLit))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	image, err := db.MarshalImage()
	if err != nil {
		log.Fatal(err)
	}
	if *check {
		committed, e := os.ReadFile(*out)
		if e != nil {
			log.Fatal(e)
		}
		if !bytes.Equal(image, committed) {
			log.Fatal("image is stale; rerun the go:generate command and commit the result")
		}
	} else if err = os.WriteFile(*out, image, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d patterns, %d bytes\n", *out, len(pats), len(image))
}
