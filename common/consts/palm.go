package consts

const SecretSalt = "sdfasdfasdfasdfjo[qwrjrioeqjopewjop23u790534u689u9R$%^&%&* &*()+"

var (
	palmVersion = ""
)

func GetPalmVersion() string {
	if palmVersion == "" {
		return "dev-insider"
	} else {
		return palmVersion
	}
}

func SetPalmVersion(t string) {
	palmVersion = t
}
