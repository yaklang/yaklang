package yakdocker

import "testing"

func TestDockerPull(t *testing.T) {
	err := pull(`rabbitmq:3-management`)
	if err != nil {
		t.Fatal(err)
	}
}
