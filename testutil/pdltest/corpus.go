// Package pdltest holds the grammar corpus shared by the parser, file reader
// and offline CLI checks. Expected validity is independent of those parsers.
package pdltest

type Case struct {
	Name, Kind, Text string
	Valid            bool
}

var Cases = []Case{
	{"policy", "policy", "grant user alice read book", true},
	{"and-or-idd", "policy", "DENY (user alice from corp, group devs), entity bot read,write 'book one' if x > 0", true},
	{"whitespace", "policy", "\tgrant\tuser\u2003alice\tread\u3000book\tif\tx > 0\r\n", true},
	{"hash", "policy", "grant user alice read 'book#1' if department == 'R#D'", true},
	{"escaped-name", "policy", `grant user "ali'ce" read 'book"one'`, true},
	{"role", "rolepolicy", "grant user alice, group devs role admin, reader on book, expr:^docs/.* if x > 0", true},
	{"role-whitespace", "rolepolicy", "grant\tuser\talice admin\ton\tbook\tif\tx > 0", true},
	{"role-hash", "rolepolicy", "grant user alice admin on 'book#1'", true},
	{"tail", "policy", "grant user alice read book garbage", false},
	{"typo-if", "policy", "grant user alice read book iff amount < 10", false},
	{"bare-if", "policy", "grant user alice read book if", false},
	{"empty-if", "policy", "grant user alice read book if  ", false},
	{"paren-tail", "policy", "grant user alice read book)", false},
	{"open-quote", "policy", "grant user alice read 'book", false},
	{"open-principal", "policy", "grant user 'alice read book", false},
	{"empty-principal", "policy", "grant user '' read book", false},
	{"role-tail", "rolepolicy", "grant user alice admin garbage", false},
	{"role-typo-if", "rolepolicy", "grant user alice admin iff x > 0", false},
	{"role-bare-if", "rolepolicy", "grant user alice admin if", false},
	{"role-open-quote", "rolepolicy", "grant user alice admin on 'book", false},
	{"role-empty-resource", "rolepolicy", "grant user alice admin on", false},
	{"role-and", "rolepolicy", "grant (user alice, group devs) admin", false},
	{"invalid-condition", "policy", "grant user alice read book if x === 1", false},
}
