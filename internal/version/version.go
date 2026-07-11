package version

const product = "MewCode"

var Value = "dev"

func String() string {
	return product + " " + Value
}
