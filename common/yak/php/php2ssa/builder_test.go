package php2ssa

import "testing"

func TestParseSSA_Smoking(t *testing.T) {
	ParseSSA(`<?php echo 111 ?>`, nil)
}

func TestParseSSA_Smoking2(t *testing.T) {
	ParseSSA(`<?php echo "Hello world"; // comment ?>
`, nil)
}

func TestParseSSA_1(t *testing.T) {
	ParseSSA(`<?php



?>`, nil)
}
