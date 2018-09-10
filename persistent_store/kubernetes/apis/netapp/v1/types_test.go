package v1

func MustEncode(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}

	return b
}
