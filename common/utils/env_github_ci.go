package utils

import (
	"os"
)

func CheckGithubAction() bool {
	return os.Getenv("YAKLANG_GRPCTEST_GITHUBACTION") == "true"
}
