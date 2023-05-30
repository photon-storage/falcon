package node

type sentryRule struct {
	name  string
	exact []byte
}

var (
	rules = []*sentryRule{
		&sentryRule{
			name:  "doc_write",
			exact: []byte("document.write("),
		},
		&sentryRule{
			name:  "passwd_input",
			exact: []byte("type=\"password\""),
		},
	}
)
