fritzdyn.cgi: *.go go.mod go.sum
	go build -o fritzdyn.cgi -tags cgi
install: fritzdyn.cgi
	install -c fritzdyn.cgi /usr/lib/cgi-bin
