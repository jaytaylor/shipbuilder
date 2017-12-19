package core

// TODO: Relocate this to an appropriate package.

func isTruthy(v string) bool {
	return v == "1" || v == "true" || v == "t" || v == "TRUE" || v == "T" || v == "yes" || v == "y" || v == "YES" || v == "Y"
}
