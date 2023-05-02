package domainextractor

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"yaklang.io/yaklang/common/filter"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

var singleWordDomainSuffix = filter.NewFilter()
var doubleWordDomainSuffix = filter.NewFilter()
var blackWordsInMain = filter.NewFilter()
var __singleBlockDomains = []string{"ac", "ad", "ae", "aero", "af", "ag", "ai", "al", "am", "ao", "aq", "ar", "arpa", "as", "asia", "at", "au", "aw", "ax", "az", "ba", "bb", "bd", "be", "bf", "bg", "bh", "bi", "biz", "bj", "bm", "bn", "bo", "br", "bs", "bt", "bv", "bw", "by", "bz", "ca", "cat", "cc", "cd", "cf", "cg", "ch", "ci", "ck", "cl", "cm", "cn", "co", "com", "coop", "cr", "cu", "cv", "cw", "cx", "cy", "cz", "de", "dj", "dk", "dm", "do", "dz", "ec", "edu", "ee", "eg", "er", "es", "et", "eu", "fi", "fj", "fk", "fm", "fo", "fr", "ga", "gb", "gd", "ge", "gf", "gg", "gh", "gi", "gl", "gm", "gn", "gov", "gp", "gq", "gr", "gs", "gt", "gu", "gw", "gy", "hk", "hm", "hn", "hr", "ht", "hu", "il", "im", "in", "info", "int", "io", "iq", "ir", "is", "it", "je", "jm", "jo", "jobs", "jp", "ke", "kg", "kh", "ki", "km", "kn", "kp", "kr", "kw", "ky", "kz", "la", "lb", "lc", "li", "lk", "lr", "ls", "lt", "lu", "lv", "ly", "ma", "mc", "md", "me", "mg", "mh", "mil", "mk", "ml", "mm", "mn", "mo", "mobi", "mp", "mq", "mr", "ms", "mt", "mu", "mv", "mw", "mx", "my", "mz", "na", "name", "nc", "ne", "net", "nf", "ng", "ni", "nl", "no", "np", "nr", "nu", "nz", "om", "onion", "org", "pa", "pe", "pf", "pg", "ph", "pk", "pl", "pm", "pn", "post", "pr", "pro", "ps", "pt", "pw", "py", "qa", "re", "ro", "rs", "ru", "rw", "sa", "sb", "sc", "sd", "se", "sg", "sh", "si", "sj", "sk", "sl", "sm", "sn", "so", "sr", "ss", "st", "su", "sv", "sx", "sy", "sz", "tc", "td", "tel", "tf", "tg", "th", "tj", "tk", "tl", "tm", "tn", "to", "tr", "tt", "tv", "tw", "tz", "ua", "ug", "uk", "us", "uy", "uz", "va", "vc", "ve", "vg", "vi", "vn", "vu", "wf", "ws", "yt", "бг", "ею", "ευ", "ελ", "рф", "xxx", "ye", "zm", "zw", "aaa", "aarp", "abb", "abc", "able", "aco", "actor", "ads", "adult", "aeg", "aetna", "afl", "aig", "akdn", "ally", "amex", "amfam", "amica", "anz", "aol", "app", "apple", "arab", "archi", "army", "art", "arte", "asda", "audi", "audio", "auto", "autos", "aws", "axa", "azure", "baby", "baidu", "band", "bank", "bar", "bbc", "bbt", "bbva", "bcg", "bcn", "beats", "beer", "best", "bet", "bible", "bid", "bike", "bing", "bingo", "bio", "black", "blog", "blue", "bms", "bmw", "boats", "bofa", "bom", "bond", "boo", "book", "bosch", "bot", "box", "build", "buy", "buzz", "bzh", "cab", "cafe", "cal", "cam", "camp", "canon", "car", "cards", "care", "cars", "casa", "case", "cash", "cba", "cbn", "cbre", "cbs", "ceo", "cern", "cfa", "cfd", "chase", "chat", "cheap", "cisco", "citi", "citic", "city", "click", "cloud", "club", "coach", "codes", "cool", "cpa", "crown", "crs", "cymru", "cyou", "dabur", "dad", "dance", "data", "date", "day", "dclk", "dds", "deal", "deals", "dell", "delta", "desi", "dev", "dhl", "diet", "dish", "diy", "dnp", "docs", "dog", "dot", "drive", "dtv", "dubai", "dvag", "dvr", "earth", "eat", "eco", "edeka", "email", "epson", "erni", "esq", "eus", "fage", "fail", "faith", "fan", "fans", "farm", "fast", "fedex", "fiat", "fido", "film", "final", "fire", "fish", "fit", "flir", "fly", "foo", "food", "ford", "forex", "forum", "fox", "free", "frl", "ftr", "fun", "fund", "fyi", "gal", "gallo", "game", "games", "gap", "gay", "gbiz", "gdn", "gea", "gent", "ggee", "gift", "gifts", "gives", "glass", "gle", "globo", "gmail", "gmbh", "gmo", "gmx", "gold", "golf", "goo", "goog", "gop", "got", "green", "gripe", "group", "gucci", "guge", "guide", "guru", "hair", "haus", "hbo", "hdfc", "help", "here", "hgtv", "hiv", "hkt", "homes", "honda", "horse", "host", "hot", "house", "how", "hsbc", "hyatt", "ibm", "icbc", "ice", "icu", "ieee", "ifm", "ikano", "imdb", "immo", "inc", "ing", "ink", "irish", "ist", "itau", "itv", "java", "jcb", "jeep", "jetzt", "jio", "jll", "jmp", "jnj", "jot", "joy", "jprs", "kddi", "kfh", "kia", "kids", "kim", "kiwi", "koeln", "kpmg", "kpn", "krd", "kred", "kyoto", "lamer", "land", "lat", "law", "lds", "lease", "legal", "lego", "lexus", "lgbt", "lidl", "life", "like", "lilly", "limo", "linde", "link", "lipsy", "live", "llc", "llp", "loan", "loans", "locus", "loft", "lol", "lotte", "lotto", "love", "lpl", "ltd", "ltda", "luxe", "macys", "maif", "man", "mango", "mba", "med", "media", "meet", "meme", "men", "menu", "miami", "mini", "mint", "mit", "mlb", "mls", "mma", "moda", "moe", "moi", "mom", "money", "moto", "mov", "movie", "msd", "mtn", "mtr", "music", "nab", "navy", "nba", "nec", "new", "news", "next", "nexus", "nfl", "ngo", "nhk", "nico", "nike", "nikon", "ninja", "nokia", "nowtv", "nra", "nrw", "ntt", "nyc", "obi", "ollo", "omega", "one", "ong", "onl", "ooo", "open", "osaka", "ott", "ovh", "page", "paris", "pars", "parts", "party", "pay", "pccw", "pet", "phd", "phone", "photo", "pics", "pid", "pin", "ping", "pink", "pizza", "place", "play", "plus", "pnc", "pohl", "poker", "porn", "praxi", "press", "prime", "prod", "prof", "promo", "pru", "pub", "pwc", "qpon", "quest", "radio", "read", "red", "rehab", "reise", "reit", "ren", "rent", "rest", "rich", "ricoh", "ril", "rio", "rip", "rocks", "rodeo", "room", "rsvp", "rugby", "ruhr", "run", "rwe", "safe", "sale", "salon", "sap", "sarl", "sas", "save", "saxo", "sbi", "sbs", "sca", "scb", "scot", "seat", "seek", "sener", "ses", "seven", "sew", "sex", "sexy", "sfr", "sharp", "shaw", "shell", "shia", "shoes", "shop", "show", "silk", "sina", "site", "ski", "skin", "sky", "skype", "sling", "smart", "smile", "sncf", "sohu", "solar", "song", "sony", "soy", "spa", "space", "sport", "spot", "srl", "stada", "star", "stc", "store", "study", "surf", "swiss", "tab", "talk", "tatar", "tax", "taxi", "tci", "tdk", "team", "tech", "teva", "thd", "tiaa", "tips", "tires", "tirol", "tjx", "tmall", "today", "tokyo", "tools", "top", "toray", "total", "tours", "town", "toys", "trade", "trust", "trv", "tube", "tui", "tunes", "tushu", "tvs", "ubank", "ubs", "uno", "uol", "ups", "vana", "vegas", "vet", "video", "vig", "vin", "vip", "visa", "viva", "vivo", "vodka", "volvo", "vote", "voto", "wales", "wang", "watch", "weber", "weibo", "weir", "wien", "wiki", "win", "wine", "wme", "work", "works", "world", "wow", "wtc", "wtf", "xbox", "xerox", "xin", "xyz", "yahoo", "yoga", "you", "yun", "zara", "zero", "zip", "zone"}
var __doubleBlockDomains = []string{"com.ac", "edu.ac", "gov.ac", "net.ac", "mil.ac", "org.ac", "nom.ad", "co.ae", "net.ae", "org.ae", "sch.ae", "ac.ae", "gov.ae", "mil.ae", "gov.af", "com.af", "org.af", "net.af", "edu.af", "com.ag", "org.ag", "net.ag", "co.ag", "nom.ag", "com.al", "edu.al", "gov.al", "mil.al", "net.al", "org.al", "co.am", "com.am", "net.am", "org.am", "ed.ao", "gv.ao", "og.ao", "co.ao", "pb.ao", "it.ao", "bet.ar", "com.ar", "coop.ar", "edu.ar", "gob.ar", "gov.ar", "int.ar", "mil.ar", "net.ar", "org.ar", "tur.ar", "iris.arpa", "uri.arpa", "urn.arpa", "gov.as", "ac.at", "co.at", "gv.at", "or.at", "com.au", "net.au", "org.au", "edu.au", "gov.au", "asn.au", "id.au", "info.au", "conf.au", "oz.au", "act.au", "nsw.au", "nt.au", "qld.au", "sa.au", "tas.au", "vic.au", "wa.au", "com.aw", "com.az", "net.az", "int.az", "gov.az", "org.az", "edu.az", "info.az", "pp.az", "mil.az", "name.az", "pro.az", "biz.az", "com.ba", "edu.ba", "gov.ba", "mil.ba", "net.ba", "org.ba", "biz.bb", "co.bb", "com.bb", "edu.bb", "gov.bb", "info.bb", "net.bb", "org.bb", "store.bb", "tv.bb", "ac.be", "gov.bf", "a.bg", "b.bg", "c.bg", "d.bg", "e.bg", "f.bg", "g.bg", "h.bg", "i.bg", "j.bg", "k.bg", "l.bg", "m.bg", "n.bg", "o.bg", "p.bg", "q.bg", "r.bg", "s.bg", "t.bg", "u.bg", "v.bg", "w.bg", "x.bg", "y.bg", "z.bg", "com.bh", "edu.bh", "net.bh", "org.bh", "gov.bh", "co.bi", "com.bi", "edu.bi", "or.bi", "org.bi", "asso.bj", "gouv.bj", "com.bm", "edu.bm", "gov.bm", "net.bm", "org.bm", "com.bn", "edu.bn", "gov.bn", "net.bn", "org.bn", "com.bo", "edu.bo", "gob.bo", "int.bo", "org.bo", "net.bo", "mil.bo", "tv.bo", "web.bo", "agro.bo", "arte.bo", "blog.bo", "info.bo", "salud.bo", "tksat.bo", "wiki.bo", "com.bt", "edu.bt", "gov.bt", "net.bt", "org.bt", "co.bw", "org.bw", "gov.by", "mil.by", "com.by", "of.by", "com.bz", "net.bz", "org.bz", "edu.bz", "gov.bz", "ab.ca", "bc.ca", "mb.ca", "nb.ca", "nf.ca", "nl.ca", "ns.ca", "nt.ca", "nu.ca", "on.ca", "pe.ca", "qc.ca", "sk.ca", "yk.ca", "gc.ca", "gov.cd", "org.ci", "or.ci", "com.ci", "co.ci", "edu.ci", "ed.ci", "ac.ci", "net.ci", "go.ci", "asso.ci", "int.ci", "md.ci", "gouv.ci", "co.cl", "gob.cl", "gov.cl", "mil.cl", "co.cm", "com.cm", "gov.cm", "net.cm", "ac.cn", "com.cn", "edu.cn", "gov.cn", "net.cn", "org.cn", "mil.cn", "ah.cn", "bj.cn", "cq.cn", "fj.cn", "gd.cn", "gs.cn", "gz.cn", "gx.cn", "ha.cn", "hb.cn", "he.cn", "hi.cn", "hl.cn", "hn.cn", "jl.cn", "js.cn", "jx.cn", "ln.cn", "nm.cn", "nx.cn", "qh.cn", "sc.cn", "sd.cn", "sh.cn", "sn.cn", "sx.cn", "tj.cn", "xj.cn", "xz.cn", "yn.cn", "zj.cn", "hk.cn", "mo.cn", "tw.cn", "arts.co", "com.co", "edu.co", "firm.co", "gov.co", "info.co", "int.co", "mil.co", "net.co", "nom.co", "org.co", "rec.co", "web.co", "ac.cr", "co.cr", "ed.cr", "fi.cr", "go.cr", "or.cr", "sa.cr", "com.cu", "edu.cu", "org.cu", "net.cu", "gov.cu", "inf.cu", "com.cv", "edu.cv", "int.cv", "nome.cv", "org.cv", "com.cw", "edu.cw", "net.cw", "org.cw", "gov.cx", "ac.cy", "biz.cy", "com.cy", "gov.cy", "ltd.cy", "mil.cy", "net.cy", "org.cy", "press.cy", "pro.cy", "tm.cy", "com.dm", "net.dm", "org.dm", "edu.dm", "gov.dm", "art.do", "com.do", "edu.do", "gob.do", "gov.do", "mil.do", "net.do", "org.do", "sld.do", "web.do", "art.dz", "asso.dz", "com.dz", "edu.dz", "gov.dz", "org.dz", "net.dz", "pol.dz", "soc.dz", "tm.dz", "com.ec", "info.ec", "net.ec", "fin.ec", "med.ec", "pro.ec", "org.ec", "edu.ec", "gov.ec", "gob.ec", "mil.ec", "edu.ee", "gov.ee", "riik.ee", "lib.ee", "med.ee", "com.ee", "pri.ee", "aip.ee", "org.ee", "fie.ee", "com.eg", "edu.eg", "eun.eg", "gov.eg", "mil.eg", "name.eg", "net.eg", "org.eg", "sci.eg", "com.es", "nom.es", "org.es", "gob.es", "edu.es", "com.et", "gov.et", "org.et", "edu.et", "biz.et", "name.et", "info.et", "net.et", "aland.fi", "ac.fj", "biz.fj", "com.fj", "gov.fj", "info.fj", "mil.fj", "name.fj", "net.fj", "org.fj", "pro.fj", "com.fm", "edu.fm", "net.fm", "org.fm", "asso.fr", "com.fr", "gouv.fr", "nom.fr", "prd.fr", "tm.fr", "cci.fr", "greta.fr", "port.fr", "edu.gd", "gov.gd", "com.ge", "edu.ge", "gov.ge", "org.ge", "mil.ge", "net.ge", "pvt.ge", "com.gh", "edu.gh", "gov.gh", "org.gh", "mil.gh", "com.gi", "ltd.gi", "gov.gi", "mod.gi", "edu.gi", "org.gi", "co.gl", "com.gl", "edu.gl", "net.gl", "org.gl", "ac.gn", "com.gn", "edu.gn", "gov.gn", "org.gn", "net.gn", "com.gp", "net.gp", "mobi.gp", "edu.gp", "org.gp", "asso.gp", "com.gr", "edu.gr", "net.gr", "org.gr", "gov.gr", "com.gt", "edu.gt", "gob.gt", "ind.gt", "mil.gt", "net.gt", "org.gt", "com.gu", "edu.gu", "gov.gu", "guam.gu", "info.gu", "net.gu", "org.gu", "web.gu", "co.gy", "com.gy", "edu.gy", "gov.gy", "net.gy", "org.gy", "com.hk", "edu.hk", "gov.hk", "idv.hk", "net.hk", "org.hk", "com.hn", "edu.hn", "org.hn", "net.hn", "mil.hn", "gob.hn", "iz.hr", "from.hr", "name.hr", "com.hr", "com.ht", "shop.ht", "firm.ht", "info.ht", "adult.ht", "net.ht", "pro.ht", "org.ht", "med.ht", "art.ht", "coop.ht", "pol.ht", "asso.ht", "edu.ht", "rel.ht", "gouv.ht", "perso.ht", "ac.id", "biz.id", "co.id", "desa.id", "go.id", "mil.id", "my.id", "net.id", "or.id", "sch.id", "web.id", "gov.ie", "ac.il", "co.il", "gov.il", "idf.il", "muni.il", "net.il", "org.il", "ac.im", "co.im", "com.im", "net.im", "org.im", "tt.im", "tv.im", "ac.in", "ai.in", "am.in", "bihar.in", "biz.in", "ca.in", "cn.in", "co.in", "com.in", "coop.in", "cs.in", "delhi.in", "dr.in", "edu.in", "er.in", "firm.in", "gen.in", "gov.in", "ind.in", "info.in", "int.in", "io.in", "me.in", "mil.in", "net.in", "nic.in", "org.in", "pg.in", "post.in", "pro.in", "res.in", "tv.in", "uk.in", "up.in", "us.in", "eu.int", "gov.iq", "edu.iq", "mil.iq", "com.iq", "org.iq", "net.iq", "ac.ir", "co.ir", "gov.ir", "id.ir", "net.ir", "org.ir", "sch.ir", "net.is", "com.is", "edu.is", "gov.is", "org.is", "int.is", "co.je", "net.je", "org.je", "com.jo", "org.jo", "net.jo", "edu.jo", "sch.jo", "gov.jo", "mil.jo", "name.jo", "ac.ke", "co.ke", "go.ke", "info.ke", "me.ke", "mobi.ke", "ne.ke", "or.ke", "sc.ke", "org.kg", "net.kg", "com.kg", "edu.kg", "gov.kg", "mil.kg", "edu.ki", "biz.ki", "net.ki", "org.ki", "gov.ki", "info.ki", "com.ki", "org.km", "nom.km", "gov.km", "prd.km", "tm.km", "edu.km", "mil.km", "ass.km", "com.km", "coop.km", "asso.km", "gouv.km", "net.kn", "org.kn", "edu.kn", "gov.kn", "com.kp", "edu.kp", "gov.kp", "org.kp", "rep.kp", "tra.kp", "ac.kr", "co.kr", "es.kr", "go.kr", "hs.kr", "kg.kr", "mil.kr", "ms.kr", "ne.kr", "or.kr", "pe.kr", "re.kr", "sc.kr", "busan.kr", "daegu.kr", "jeju.kr", "seoul.kr", "ulsan.kr", "com.kw", "edu.kw", "emb.kw", "gov.kw", "ind.kw", "net.kw", "org.kw", "com.ky", "edu.ky", "net.ky", "org.ky", "org.kz", "edu.kz", "net.kz", "gov.kz", "mil.kz", "com.kz", "int.la", "net.la", "info.la", "edu.la", "gov.la", "per.la", "com.la", "org.la", "com.lb", "edu.lb", "gov.lb", "net.lb", "org.lb", "com.lc", "net.lc", "co.lc", "org.lc", "edu.lc", "gov.lc", "gov.lk", "sch.lk", "net.lk", "int.lk", "com.lk", "org.lk", "edu.lk", "ngo.lk", "soc.lk", "web.lk", "ltd.lk", "assn.lk", "grp.lk", "hotel.lk", "ac.lk", "com.lr", "edu.lr", "gov.lr", "org.lr", "net.lr", "ac.ls", "biz.ls", "co.ls", "edu.ls", "gov.ls", "info.ls", "net.ls", "org.ls", "sc.ls", "gov.lt", "com.lv", "edu.lv", "gov.lv", "org.lv", "mil.lv", "id.lv", "net.lv", "asn.lv", "conf.lv", "com.ly", "net.ly", "gov.ly", "plc.ly", "edu.ly", "sch.ly", "med.ly", "org.ly", "id.ly", "co.ma", "net.ma", "gov.ma", "org.ma", "ac.ma", "press.ma", "tm.mc", "asso.mc", "org.mg", "nom.mg", "gov.mg", "prd.mg", "tm.mg", "edu.mg", "mil.mg", "com.mg", "co.mg", "com.mk", "org.mk", "net.mk", "edu.mk", "gov.mk", "inf.mk", "name.mk", "com.ml", "edu.ml", "gouv.ml", "gov.ml", "net.ml", "org.ml", "gov.mn", "edu.mn", "org.mn", "com.mo", "net.mo", "org.mo", "edu.mo", "gov.mo", "gov.mr", "com.ms", "edu.ms", "gov.ms", "net.ms", "org.ms", "com.mt", "edu.mt", "net.mt", "org.mt", "com.mu", "net.mu", "org.mu", "gov.mu", "ac.mu", "co.mu", "or.mu", "aero.mv", "biz.mv", "com.mv", "coop.mv", "edu.mv", "gov.mv", "info.mv", "int.mv", "mil.mv", "name.mv", "net.mv", "org.mv", "pro.mv", "ac.mw", "biz.mw", "co.mw", "com.mw", "coop.mw", "edu.mw", "gov.mw", "int.mw", "net.mw", "org.mw", "com.mx", "org.mx", "gob.mx", "edu.mx", "net.mx", "biz.my", "com.my", "edu.my", "gov.my", "mil.my", "name.my", "net.my", "org.my", "ac.mz", "adv.mz", "co.mz", "edu.mz", "gov.mz", "mil.mz", "net.mz", "org.mz", "info.na", "pro.na", "name.na", "or.na", "dr.na", "us.na", "mx.na", "ca.na", "in.na", "cc.na", "tv.na", "ws.na", "mobi.na", "co.na", "com.na", "org.na", "asso.nc", "nom.nc", "com.nf", "net.nf", "per.nf", "rec.nf", "web.nf", "arts.nf", "firm.nf", "info.nf", "other.nf", "store.nf", "com.ng", "edu.ng", "gov.ng", "i.ng", "mil.ng", "mobi.ng", "name.ng", "net.ng", "org.ng", "sch.ng", "ac.ni", "biz.ni", "co.ni", "com.ni", "edu.ni", "gob.ni", "in.ni", "info.ni", "int.ni", "mil.ni", "net.ni", "nom.ni", "org.ni", "web.ni", "biz.nr", "info.nr", "gov.nr", "edu.nr", "org.nr", "net.nr", "com.nr", "ac.nz", "co.nz", "cri.nz", "geek.nz", "gen.nz", "govt.nz", "iwi.nz", "kiwi.nz", "maori.nz", "mil.nz", "net.nz", "org.nz", "co.om", "com.om", "edu.om", "gov.om", "med.om", "net.om", "org.om", "pro.om", "ac.pa", "gob.pa", "com.pa", "org.pa", "sld.pa", "edu.pa", "net.pa", "ing.pa", "abo.pa", "med.pa", "nom.pa", "edu.pe", "gob.pe", "nom.pe", "mil.pe", "org.pe", "com.pe", "net.pe", "com.pf", "org.pf", "edu.pf", "com.ph", "net.ph", "org.ph", "gov.ph", "edu.ph", "ngo.ph", "mil.ph", "i.ph", "com.pk", "net.pk", "edu.pk", "org.pk", "fam.pk", "biz.pk", "web.pk", "gov.pk", "gob.pk", "gok.pk", "gon.pk", "gop.pk", "gos.pk", "info.pk", "gov.pn", "co.pn", "org.pn", "edu.pn", "net.pn", "com.pr", "net.pr", "org.pr", "gov.pr", "edu.pr", "isla.pr", "pro.pr", "biz.pr", "info.pr", "name.pr", "est.pr", "prof.pr", "ac.pr", "edu.ps", "gov.ps", "sec.ps", "plo.ps", "com.ps", "org.ps", "net.ps", "net.pt", "gov.pt", "org.pt", "edu.pt", "int.pt", "publ.pt", "com.pt", "nome.pt", "co.pw", "ne.pw", "or.pw", "ed.pw", "go.pw", "belau.pw", "com.py", "coop.py", "edu.py", "gov.py", "mil.py", "net.py", "org.py", "com.qa", "edu.qa", "gov.qa", "mil.qa", "name.qa", "net.qa", "org.qa", "sch.qa", "asso.re", "com.re", "nom.re", "arts.ro", "com.ro", "firm.ro", "info.ro", "nom.ro", "nt.ro", "org.ro", "rec.ro", "store.ro", "tm.ro", "ac.rs", "co.rs", "edu.rs", "gov.rs", "in.rs", "org.rs", "ac.rw", "co.rw", "coop.rw", "gov.rw", "mil.rw", "net.rw", "org.rw", "com.sa", "net.sa", "org.sa", "gov.sa", "med.sa", "pub.sa", "edu.sa", "sch.sa", "com.sb", "edu.sb", "gov.sb", "net.sb", "org.sb", "com.sc", "gov.sc", "net.sc", "org.sc", "edu.sc", "com.sd", "net.sd", "org.sd", "edu.sd", "med.sd", "tv.sd", "gov.sd", "info.sd", "a.se", "ac.se", "b.se", "bd.se", "brand.se", "c.se", "d.se", "e.se", "f.se", "fh.se", "fhsk.se", "fhv.se", "g.se", "h.se", "i.se", "k.se", "l.se", "m.se", "n.se", "o.se", "org.se", "p.se", "parti.se", "pp.se", "press.se", "r.se", "s.se", "t.se", "tm.se", "u.se", "w.se", "x.se", "y.se", "z.se", "com.sg", "net.sg", "org.sg", "gov.sg", "edu.sg", "per.sg", "com.sh", "net.sh", "gov.sh", "org.sh", "mil.sh", "com.sl", "net.sl", "edu.sl", "gov.sl", "org.sl", "art.sn", "com.sn", "edu.sn", "gouv.sn", "org.sn", "perso.sn", "univ.sn", "biz.ss", "com.ss", "edu.ss", "gov.ss", "me.ss", "net.ss", "org.ss", "sch.ss", "co.st", "com.st", "edu.st", "mil.st", "net.st", "org.st", "store.st", "com.sv", "edu.sv", "gob.sv", "org.sv", "red.sv", "gov.sx", "edu.sy", "gov.sy", "net.sy", "mil.sy", "com.sy", "org.sy", "co.sz", "ac.sz", "org.sz", "ac.th", "co.th", "go.th", "in.th", "mi.th", "net.th", "or.th", "ac.tj", "biz.tj", "co.tj", "com.tj", "edu.tj", "go.tj", "gov.tj", "int.tj", "mil.tj", "name.tj", "net.tj", "nic.tj", "org.tj", "test.tj", "web.tj", "gov.tl", "com.tm", "co.tm", "org.tm", "net.tm", "nom.tm", "gov.tm", "mil.tm", "edu.tm", "com.tn", "ens.tn", "fin.tn", "gov.tn", "ind.tn", "info.tn", "intl.tn", "nat.tn", "net.tn", "org.tn", "perso.tn", "av.tr", "bbs.tr", "bel.tr", "biz.tr", "com.tr", "dr.tr", "edu.tr", "gen.tr", "gov.tr", "info.tr", "mil.tr", "kep.tr", "name.tr", "net.tr", "org.tr", "pol.tr", "tel.tr", "tsk.tr", "tv.tr", "web.tr", "nc.tr", "ac.tz", "co.tz", "go.tz", "hotel.tz", "info.tz", "me.tz", "mil.tz", "mobi.tz", "ne.tz", "or.tz", "sc.tz", "tv.tz", "com.ua", "edu.ua", "gov.ua", "in.ua", "net.ua", "org.ua", "ck.ua", "cn.ua", "cr.ua", "cv.ua", "dn.ua", "dp.ua", "if.ua", "kh.ua", "kiev.ua", "km.ua", "kr.ua", "krym.ua", "ks.ua", "kv.ua", "kyiv.ua", "lg.ua", "lt.ua", "lutsk.ua", "lv.ua", "lviv.ua", "mk.ua", "od.ua", "odesa.ua", "pl.ua", "rivne.ua", "rovno.ua", "rv.ua", "sb.ua", "sm.ua", "sumy.ua", "te.ua", "uz.ua", "vn.ua", "volyn.ua", "yalta.ua", "zp.ua", "zt.ua", "co.ug", "or.ug", "ac.ug", "sc.ug", "go.ug", "ne.ug", "com.ug", "org.ug", "ac.uk", "co.uk", "gov.uk", "ltd.uk", "me.uk", "net.uk", "nhs.uk", "org.uk", "plc.uk", "sch.uk", "com.uy", "edu.uy", "gub.uy", "mil.uy", "net.uy", "org.uy", "co.uz", "com.uz", "net.uz", "org.uz", "com.vc", "net.vc", "org.vc", "gov.vc", "mil.vc", "edu.vc", "arts.ve", "bib.ve", "co.ve", "com.ve", "edu.ve", "firm.ve", "gob.ve", "gov.ve", "info.ve", "int.ve", "mil.ve", "net.ve", "nom.ve", "org.ve", "rar.ve", "rec.ve", "store.ve", "tec.ve", "web.ve", "co.vi", "com.vi", "net.vi", "org.vi", "com.vn", "net.vn", "org.vn", "edu.vn", "gov.vn", "int.vn", "ac.vn", "biz.vn", "info.vn", "name.vn", "pro.vn", "com.vu", "edu.vu", "net.vu", "org.vu", "com.ws", "net.ws", "org.ws", "gov.ws", "edu.ws", "com.ye", "edu.ye", "gov.ye", "net.ye", "mil.ye", "org.ye", "ac.za", "agric.za", "alt.za", "co.za", "edu.za", "gov.za", "law.za", "mil.za", "net.za", "ngo.za", "nic.za", "nis.za", "nom.za", "org.za", "tm.za", "web.za", "ac.zm", "biz.zm", "co.zm", "com.zm", "edu.zm", "gov.zm", "info.zm", "mil.zm", "net.zm", "org.zm", "sch.zm", "ac.zw", "co.zw", "gov.zw", "mil.zw", "org.zw", "cc.ua", "inf.ua", "ltd.ua", "gv.vc", "awdev.ca", "rs.ba", "base.ec", "base.shop", "bnr.la", "of.je", "mycd.eu", "drr.ac", "carrd.co", "crd.co", "ju.mp", "com.de", "com.se", "za.bz", "web.in", "radio.am", "radio.fm", "c.la", "cx.ua", "co.ca", "otap.co", "co.cz", "cnpy.gdn", "co.nl", "ac.ru", "edu.ru", "gov.ru", "int.ru", "mil.ru", "test.ru", "realm.cz", "dapps.earth", "jozi.biz", "shop.th", "bip.sh", "dy.fi", "ath.cx", "mine.nu", "ddnss.de", "onred.one", "bir.ru", "cbg.ru", "com.ru", "msk.ru", "mytis.ru", "nov.ru", "spb.ru", "exnet.su", "lenug.su", "msk.su", "navoi.su", "nov.su", "penza.su", "sochi.su", "spb.su", "tula.su", "tuva.su", "user.fm", "conn.uk", "copro.uk", "hosp.uk", "flap.id", "fbxos.fr", "lab.ms", "gsj.bz", "co.ro", "shop.ro", "pymnt.uk", "ro.im", "goip.de", "cloud.goog", "gov.nl", "fin.ci", "free.hr", "caa.li", "ua.rs", "conf.se", "hs.zone", "orx.biz", "biz.gl", "col.ng", "firm.ng", "gen.ng", "ltd.ng", "ngo.ng", "edu.scot", "iki.fi", "biz.at", "info.at", "info.cx", "kapsi.fi", "co.krd", "edu.krd", "co.place", "cn.vu", "mcdir.ru", "mcpre.ru", "forte.id", "net.ru", "org.ru", "pp.ru", "mypi.co", "ntdll.top", "zapto.xyz", "myftp.biz", "nyc.mn", "omg.lol", "own.pm", "owo.codes", "ox.rs", "oy.lc", "co.bn", "priv.at", "ras.ru", "repl.co", "gov.scot", "spdns.de", "spdns.eu", "biz.ua", "co.ua", "pp.ua", "storj.farm", "de.cool", "lima.zone", "uber.space", "ltd.hk", "inc.hk", "name.pm", "sch.tf", "biz.wf", "sch.wf", "org.yt", "now.sh", "neko.am", "nyaa.am", "be.ax", "cat.ax", "es.ax", "eu.ax", "gg.ax", "mc.ax", "us.ax", "xy.ax", "nl.ci", "xx.gl", "app.gp", "blog.gt", "de.gt", "to.gt", "be.gy", "cc.hn", "blog.kg", "io.kg", "jp.kg", "tv.kg", "uk.kg", "us.kg", "de.ls", "at.md", "de.md", "jp.md", "to.md", "indie.porn", "vxl.sh", "ch.tc", "me.tc", "we.tc", "at.vg", "blog.vu", "dev.vu", "me.vu", "v.ua", "demon.nl", "ynh.fr", "noho.st"}
var __blackWordsMainDomain = []string{
	"a", "css", "js", "slice", "prototype", "t", "o", "this", "f", "i", "n", "c", "date", "list",
	"base64", "div", "li", "response",
}

func init() {
	for _, d := range __singleBlockDomains {
		d = strings.TrimLeft(d, ".")
		singleWordDomainSuffix.Insert(d)
		singleWordDomainSuffix.Insert("." + d)
	}
	for _, d := range __doubleBlockDomains {
		d = strings.TrimLeft(d, ".")
		doubleWordDomainSuffix.Insert(d)
		doubleWordDomainSuffix.Insert("." + d)
	}
	for _, d := range __blackWordsMainDomain {
		d = strings.TrimLeft(d, ".")
		blackWordsInMain.Insert(d)
	}
}

func haveDomainSuffix(b []string) (string, bool) {
	rootDomain, ok := _haveDomainSuffix(b)
	if !ok {
		return rootDomain, ok
	}

	if ret := strings.Split(rootDomain, "."); len(ret) > 0 {
		if blackWordsInMain.Exist(strings.ToLower(ret[0])) {
			// 在禁用词中
			return "", false
		}
		if len(ret[0]) <= 1 {
			return "", false
		}
		originLast := ret[0][1:]
		if strings.ToLower(originLast) != originLast {
			return "", false
		}
	}
	return rootDomain, ok
}

func _haveDomainSuffix(b []string) (string, bool) {
	if len(b) <= 1 {
		return "", false
	}
	if len(b) == 2 {
		if doubleWordDomainSuffix.Exist(strings.Join(b, ".")) {
			return "", false
		}
		return strings.Trim(strings.Join(b, "."), "."), singleWordDomainSuffix.Exist(strings.TrimRight(b[1], "."))
	}

	// 3 个以及以上的话需要区分优先级
	if doubleWordDomainSuffix.Exist(strings.TrimRight(strings.Join([]string{
		b[len(b)-2], b[len(b)-1],
	}, "."), ".")) {
		return strings.TrimRight(strings.Join([]string{
			b[len(b)-3], b[len(b)-2], b[len(b)-1],
		}, "."), "."), true
	}

	if singleWordDomainSuffix.Exist(b[len(b)-1]) {
		return strings.TrimRight(strings.Join([]string{
			b[len(b)-2], b[len(b)-1],
		}, "."), "."), true
	}

	return "", false
}

func HaveDomainSuffix(b string) bool {
	if b == "" {
		return false
	}
	_, result := haveDomainSuffix(strings.Split(b, "."))
	return result
}

var (
	reDoubleUrl   = regexp.MustCompile(`(?P<durl>(%25[a-fA-F0-9]{2}){2,})`)
	reQuoted      = regexp.MustCompile(`(?P<quoted>((\\{2}x[0-9a-fA-F]{2})|(\\x[0-9a-fA-F]{2})))`)
	reJsonUnicode = regexp.MustCompile(`(?P<jsonunicode>((\\{2}u[0-9a-fA-F]{4})|(\\u[0-9a-fA-F]{4})))`)
	reUrlEncode   = regexp.MustCompile(`(?P<urlencode>((%[a-fA-F0-9]{2})+))`)
)

func TryDecode(s string) string {
	var ret string
	ret = reDoubleUrl.ReplaceAllStringFunc(s, func(i string) string {
		result, err := codec.DoubleDecodeUrl(i)
		if err != nil {
			return i
		}
		return result
	})
	if ret == "" {
		ret = s
	}
	s = ret

	ret = reQuoted.ReplaceAllStringFunc(s, func(i string) string {
		raw, err := codec.StrConvUnquote(`"` + i + `"`)
		if err != nil {
			return i
		}
		return raw
	})
	if ret == "" {
		ret = s
	}
	s = ret

	ret = reJsonUnicode.ReplaceAllStringFunc(s, func(i string) string {
		return codec.JsonUnicodeDecode(i)
	})
	if ret == "" {
		ret = s
	}
	s = ret

	ret = reUrlEncode.ReplaceAllStringFunc(s, func(i string) string {
		raw, _ := codec.QueryUnescape(i)
		if raw == "" {
			return i
		}
		return raw
	})
	if ret == "" {
		ret = s
	}
	s = ret

	return s
}

func ExtractDomains(code string) []string {
	var results, rootDomains = scan(code)
	return utils.RemoveRepeatStringSlice(append(rootDomains, results...))
}

func ExtractRootDomains(code string) []string {
	var results, rootDomains = scan(code)
	var filters = filter.NewFilter()
	var ret []string
	for _, i := range append(rootDomains, results...) {
		r := ExtractRootDomain(i)
		if filters.Exist(r) {
			continue
		}
		filters.Insert(r)
		ret = append(ret, r)
	}
	return ret
}

func ExtractDomainsEx(code string) ([]string, []string) {
	re1, re2 := scan(code)
	var filters = filter.NewFilter()
	var ret []string
	for _, i := range append(re1, re2...) {
		r := ExtractRootDomain(i)
		if filters.Exist(r) {
			continue
		}
		filters.Insert(r)
		ret = append(ret, r)
	}
	return re1, ret
}

func scan(code string) ([]string, []string) {
	/*
		1st: (?P<durl>(%25[a-fA-F0-9]{2}){2,})
		2nd: (?P<quoted>(([\\]{2}x[0-9a-fA-F]{2})|([\\]{1}x[0-9a-fA-F]{2})))
		3rd: (?P<jsonunicode>(([\\]{2}u[0-9a-fA-F]{4,5})|([\\]{1}u[0-9a-fA-F]{4,5})))
		4th: (?P<urlencode>((%[a-fA-F0-9]{2})+))

		html 实体编码实际上不需要处理，标准情况的话，实体编码前后都有 ;
	*/
	// 多种编码需要处理
	// 1. %[0-9a-fA-F]{2}
	// 2. \u[0-9a-fA-F]{4};?
	// 3. &#x[0-9a-fA-F]{4};?
	// 4. &#[0-9]{1,5};
	// 5. %25{\d}{2} 有多个的时候，一般这个会有用
	// 6. \?\x[0-9a-fA-F]{2}
	return _scan(TryDecode(code))
}

func _scan(code string) ([]string, []string) {
	scanner := bufio.NewScanner(bytes.NewBufferString(code))
	scanner.Split(bufio.ScanBytes)

	var lastCh byte
	var ch byte
	var blockSeries []string
	var currentBlock string

	var isDomainChar = func(c byte) bool {
		return (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			c == '-'
	}
	var results []string
	var rootDomains []string
	var resultsFilter = filter.NewFilter()
	var rootDomainFilter = filter.NewFilter()
	addResult := func(d string) {
		if resultsFilter.Exist(d) {
			return
		}
		resultsFilter.Insert(d)
		results = append(results, d)

		rootDomain := ExtractRootDomain(d)
		if rootDomain == d {
			return
		}
		if rootDomainFilter.Exist(rootDomain) {
			return
		}
		rootDomainFilter.Insert(rootDomain)
		rootDomains = append(rootDomains, rootDomain)
	}
	for {
		lastCh = ch
		if !scanner.Scan() && len(scanner.Bytes()) <= 0 {
			break
		}
		ch = scanner.Bytes()[0]

		var validDomainChar = isDomainChar(ch)

		if ch == '.' && currentBlock != "" {
			// block end
			blockSeries = append(blockSeries, currentBlock)
			currentBlock = ""
			continue
		}

		if validDomainChar {
			currentBlock += fmt.Sprintf("%c", ch)
			continue
		}

		if currentBlock != "" {
			blockSeries = append(blockSeries, currentBlock)
			currentBlock = ""
		} else {
			currentBlock = ""
		}

		_, ret := haveDomainSuffix(blockSeries)
		if len(blockSeries) > 1 && ret {
			addResult(strings.Join(blockSeries, "."))
		}
		blockSeries = nil
	}
	if currentBlock != "" {
		blockSeries = append(blockSeries, currentBlock)
	}
	_, result := haveDomainSuffix(blockSeries)
	if len(blockSeries) > 1 && result {
		addResult(strings.Join(blockSeries, "."))
	}
	_ = lastCh
	return results, rootDomains
}

func ExtractRootDomain(i string) string {
	if i == "" {
		return ""
	}
	rootDomain, result := haveDomainSuffix(strings.Split(strings.Trim(i, "."), "."))
	if result {
		return rootDomain
	}
	return i
}
