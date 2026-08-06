package bbcode

func kelimeSeti(kelimeler ...string) map[string]bool {
	m := make(map[string]bool, len(kelimeler))
	for _, k := range kelimeler {
		m[k] = true
	}
	return m
}

var (
	kwPython = kelimeSeti(
		"def", "return", "if", "elif", "else", "for", "while", "import", "from", "as",
		"class", "try", "except", "finally", "with", "pass", "break", "continue",
		"lambda", "yield", "in", "is", "not", "and", "or", "None", "True", "False",
		"self", "global", "nonlocal", "raise", "assert", "del", "async", "await", "print",
	)

	kwGo = kelimeSeti(
		"func", "return", "if", "else", "for", "range", "import", "package", "var",
		"const", "type", "struct", "interface", "map", "chan", "go", "defer", "select",
		"switch", "case", "default", "break", "continue", "nil", "true", "false",
		"make", "new", "len", "cap", "append", "panic", "recover", "fallthrough", "goto",
	)

	kwJavaScript = kelimeSeti(
		"function", "return", "if", "else", "for", "while", "var", "let", "const",
		"import", "export", "from", "class", "extends", "new", "this", "try", "catch",
		"finally", "throw", "switch", "case", "default", "break", "continue", "null",
		"undefined", "true", "false", "async", "await", "typeof", "instanceof", "in",
		"of", "static", "super", "interface", "type", "enum", "implements",
	)

	kwCLike = kelimeSeti(
		"int", "float", "double", "char", "void", "long", "short", "unsigned", "signed",
		"struct", "typedef", "return", "if", "else", "for", "while", "do", "switch",
		"case", "default", "break", "continue", "const", "static", "public", "private",
		"protected", "class", "new", "delete", "namespace", "using", "include",
		"define", "null", "nullptr", "true", "false", "this", "virtual", "override",
		"template", "try", "catch", "throw", "final", "interface", "extends",
		"implements", "import", "package", "abstract",
	)

	kwRust = kelimeSeti(
		"fn", "let", "mut", "return", "if", "else", "for", "while", "loop", "match",
		"struct", "enum", "impl", "trait", "pub", "use", "mod", "crate", "self", "Self",
		"true", "false", "None", "Some", "Ok", "Err", "as", "in", "break", "continue",
		"static", "const", "unsafe", "where", "move", "async", "await",
	)
)

// diller, [code=dil] etiketindeki ismi ve yaygın takma adları
// karşılık gelen anahtar kelime setine eşler. Yeni bir dil eklemek için:
// yukarıya `kw<Dil> = kelimeSeti(...)` ile bir anahtar kelime seti tanımla,
// buraya dilin adını/takma adlarını ekle, gerekiyorsa kwGenel'in
// birlestir(...) listesine de ekle.
var diller = map[string]map[string]bool{
	"python": kwPython, "py": kwPython, "python3": kwPython,
	"go": kwGo, "golang": kwGo,
	"javascript": kwJavaScript, "js": kwJavaScript, "typescript": kwJavaScript, "ts": kwJavaScript,
	"c": kwCLike, "cpp": kwCLike, "c++": kwCLike, "cplusplus": kwCLike,
	"java": kwCLike, "cs": kwCLike, "csharp": kwCLike, "c#": kwCLike,
	"rust": kwRust, "rs": kwRust,
}

var kwGenel = birlestir(kwPython, kwGo, kwJavaScript, kwCLike, kwRust)

func birlestir(setler ...map[string]bool) map[string]bool {
	m := make(map[string]bool)
	for _, s := range setler {
		for k := range s {
			m[k] = true
		}
	}
	return m
}

// anahtarKelimeler, verilen dil adı için vurgulanacak anahtar kelime
// setini döndürür; dil boş ya da tanınmıyorsa yaygın dillerin
// birleşimini kullanır.
func anahtarKelimeler(dil string) map[string]bool {
	if kw, ok := diller[dil]; ok {
		return kw
	}
	return kwGenel
}
