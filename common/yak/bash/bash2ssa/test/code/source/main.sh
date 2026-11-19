println() { :; }
set -- caller
. ./lib/colors.sh supplied
println "$color"
println "$1"
paint sky
source ./lib/colors.sh again
println "$color"
