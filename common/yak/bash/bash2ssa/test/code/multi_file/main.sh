println() { :; }

source ./lib/util.sh

wrapper() {
	local v=$1
	println "wrapped:$v"
}

wrapper "$(from_lib)"
println "$?"
