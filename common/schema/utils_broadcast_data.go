package schema

var boardcastData func(typeString string, data any)

func SetBoardCast_Data(f func(typeString string, data any)) {
	boardcastData = f
}
