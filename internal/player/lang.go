package player

import "strings"

// languages maps ISO 639-1 and 639-2 (B and T) codes to English names,
// for the languages track tags commonly use.
var languages = map[string]string{}

func init() {
	for _, l := range [][]string{
		{"English", "en", "eng"}, {"Arabic", "ar", "ara"}, {"Bengali", "bn", "ben"},
		{"Bulgarian", "bg", "bul"}, {"Catalan", "ca", "cat"}, {"Chinese", "zh", "chi", "zho"},
		{"Croatian", "hr", "hrv"}, {"Czech", "cs", "cze", "ces"}, {"Danish", "da", "dan"},
		{"Dutch", "nl", "dut", "nld"}, {"Estonian", "et", "est"}, {"Finnish", "fi", "fin"},
		{"French", "fr", "fre", "fra"}, {"German", "de", "ger", "deu"}, {"Greek", "el", "gre", "ell"},
		{"Hebrew", "he", "heb"}, {"Hindi", "hi", "hin"}, {"Hungarian", "hu", "hun"},
		{"Icelandic", "is", "ice", "isl"}, {"Indonesian", "id", "ind"}, {"Italian", "it", "ita"},
		{"Japanese", "ja", "jpn"}, {"Korean", "ko", "kor"}, {"Latvian", "lv", "lav"},
		{"Lithuanian", "lt", "lit"}, {"Malay", "ms", "may", "msa"}, {"Norwegian", "no", "nor"},
		{"Norwegian Bokmål", "nb", "nob"}, {"Persian", "fa", "per", "fas"}, {"Polish", "pl", "pol"},
		{"Portuguese", "pt", "por"}, {"Romanian", "ro", "rum", "ron"}, {"Russian", "ru", "rus"},
		{"Serbian", "sr", "srp"}, {"Slovak", "sk", "slo", "slk"}, {"Slovenian", "sl", "slv"},
		{"Spanish", "es", "spa"}, {"Swedish", "sv", "swe"}, {"Tamil", "ta", "tam"},
		{"Telugu", "te", "tel"}, {"Thai", "th", "tha"}, {"Turkish", "tr", "tur"},
		{"Ukrainian", "uk", "ukr"}, {"Urdu", "ur", "urd"}, {"Vietnamese", "vi", "vie"},
		{"Filipino", "fil"}, {"Kurdish", "ku", "kur"}, {"Swahili", "sw", "swa"},
		{"Undetermined", "und"},
	} {
		for _, code := range l[1:] {
			languages[code] = l[0]
		}
	}
}

// LanguageName returns the English name of a language code, or the code
// itself when unknown. Regional tags ("pt-BR") keep their region.
func LanguageName(code string) string {
	base, region, _ := strings.Cut(strings.ToLower(code), "-")
	name, ok := languages[base]
	if !ok {
		return code
	}
	if region != "" {
		return name + " (" + strings.ToUpper(region) + ")"
	}
	return name
}
