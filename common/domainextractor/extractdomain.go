package domainextractor

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"golang.org/x/exp/slices"
)

var (
	singleWordDomainSuffix = filter.NewFilterWithSize(14, 1<<15)
	doubleWordDomainSuffix = filter.NewFilterWithSize(14, 1<<15)
	blackWordsInMain       = filter.NewFilterWithSize(14, 1<<15)
	__singleBlockDomains   = []string{"aaa", "aarp", "abb", "abc", "able", "ac", "accn", "aco", "actor", "ad", "ads", "adult", "ae", "aeg", "aero", "aetna", "af", "afl", "ag", "ahcn", "ai", "aig", "akdn", "al", "ally", "am", "amex", "amfam", "amica", "anquan", "anz", "ao", "aol", "app", "apple", "aq", "ar", "arab", "archi", "archi", "army", "arpa", "art", "art", "arte", "as", "asda", "asia", "asia", "at", "au", "audi", "audio", "auto", "auto", "autos", "autos", "aw", "aws", "ax", "axa", "az", "azure", "ba", "baby", "baby", "baidu", "baidu", "band", "band", "bank", "bar", "bb", "bbc", "bbt", "bbva", "bcg", "bcn", "bd", "be", "beats", "beauty", "beer", "beer", "best", "bet", "bf", "bg", "bh", "bi", "bible", "bid", "bike", "bing", "bingo", "bio", "bio", "biz", "biz", "bj", "bjcn", "black", "black", "blog", "blue", "blue", "bm", "bms", "bmw", "bn", "bo", "boats", "boats", "bofa", "bom", "bond", "bond", "boo", "book", "bosch", "bot", "box", "br", "bs", "bt", "build", "buy", "buzz", "bv", "bw", "by", "bz", "bzh", "ca", "cab", "cab", "cafe", "cafe", "cal", "cam", "camp", "canon", "car", "car", "cards", "care", "cars", "cars", "casa", "case", "cash", "cash", "cat", "cba", "cbn", "cbre", "cbs", "cc", "cc", "cd", "center", "ceo", "cern", "cf", "cfa", "cfd", "cg", "ch", "chase", "chase", "chat", "chat", "cheap", "ci", "cisco", "citi", "citic", "citic", "city", "city", "ck", "cl", "click", "click", "cloud", "cloud", "club", "club", "cm", "cn", "cn", "co", "co", "coach", "codes", "college", "com", "com", "comcn", "company", "cool", "cool", "coop", "cpa", "cqcn", "cr", "crown", "crs", "cu", "cv", "cw", "cx", "cy", "cymru", "cyou", "cyou", "cz", "dabur", "dad", "dance", "data", "date", "day", "dclk", "dds", "de", "deal", "deals", "dell", "delta", "desi", "design", "dev", "dhl", "diet", "dish", "diy", "dj", "dk", "dm", "dnp", "do", "docs", "dog", "dot", "drive", "dtv", "dubai", "dvag", "dvr", "dz", "earth", "eat", "ec", "eco", "edeka", "edu", "educn", "ee", "eg", "email", "email", "epson", "er", "erni", "es", "esq", "et", "eu", "eus", "fage", "fail", "faith", "fan", "fan", "fans", "fans", "farm", "fashion", "fast", "fedex", "fi", "fiat", "fido", "film", "final", "fire", "fish", "fit", "fit", "fj", "fjcn", "fk", "flir", "fly", "fm", "fo", "foo", "food", "ford", "forex", "forum", "fox", "fr", "free", "frl", "ftr", "fun", "fun", "fund", "fund", "fyi", "fyi", "ga", "gal", "gallo", "game", "games", "games", "gap", "gay", "gb", "gbiz", "gd", "gdcn", "gdn", "ge", "gea", "gent", "gf", "gg", "ggee", "gh", "gi", "gift", "gifts", "gives", "gl", "glass", "gle", "global", "globo", "gm", "gmail", "gmbh", "gmo", "gmx", "gn", "gold", "gold", "golf", "goo", "goog", "gop", "got", "gov", "govcn", "gp", "gq", "gr", "green", "green", "gripe", "group", "group", "gs", "gt", "gu", "gucci", "guge", "guide", "guru", "guru", "gw", "gxcn", "gy", "gzcn", "hacn", "hair", "hair", "haus", "hbcn", "hbo", "hdfc", "hecn", "help", "here", "hgtv", "hicn", "hiv", "hk", "hk", "hkcn", "hkt", "hlcn", "hm", "hn", "hncn", "homes", "homes", "honda", "horse", "host", "host", "hot", "house", "how", "hr", "hsbc", "ht", "hu", "hyatt", "ibm", "icbc", "ice", "icu", "icu", "ieee", "ifm", "ikano", "il", "im", "imdb", "immo", "in", "inc", "info", "info", "ing", "ink", "ink", "int", "io", "iq", "ir", "irish", "is", "ist", "it", "itau", "itv", "java", "jcb", "je", "jeep", "jetzt", "jio", "jlcn", "jll", "jm", "jmp", "jnj", "jo", "jobs", "jot", "joy", "jp", "jpmorgan", "jprs", "jscn", "jxcn", "kddi", "ke", "kfh", "kg", "kh", "ki", "kia", "kids", "kids", "kim", "kim", "kiwi", "km", "kn", "koeln", "kp", "kpmg", "kpn", "kr", "krd", "kred", "kw", "ky", "kyoto", "kz", "la", "lamer", "land", "lat", "law", "law", "lb", "lc", "lds", "lease", "legal", "lego", "lexus", "lgbt", "li", "lidl", "life", "life", "like", "lilly", "limo", "linde", "link", "link", "lipsy", "live", "live", "lk", "llc", "llp", "lncn", "loan", "loans", "locus", "loft", "lol", "lotte", "lotto", "lotto", "love", "love", "lpl", "lr", "ls", "lt", "ltd", "ltd", "ltda", "lu", "luxe", "luxe", "lv", "ly", "ma", "macys", "maif", "makeup", "man", "mango", "market", "mba", "mba", "mc", "md", "me", "me", "med", "media", "media", "meet", "meme", "men", "menu", "mg", "mh", "miami", "mil", "milcn", "mini", "mint", "mit", "mk", "ml", "mlb", "mls", "mm", "mma", "mn", "mo", "mobi", "mobi", "mocn", "moda", "moe", "moi", "mom", "money", "monster", "moto", "motorcycles", "mov", "movie", "mp", "mq", "mr", "ms", "msd", "mt", "mtn", "mtr", "mu", "music", "mv", "mw", "mx", "my", "mz", "na", "nab", "name", "navy", "nba", "nc", "ne", "nec", "net", "net", "netcn", "new", "news", "news", "next", "nexus", "nf", "nfl", "ng", "ngo", "nhk", "ni", "nico", "nike", "nikon", "ninja", "nl", "nmcn", "no", "nokia", "nowtv", "np", "nr", "nra", "nrw", "ntt", "nu", "nxcn", "nyc", "nz", "obi", "ollo", "om", "omega", "one", "ong", "onion", "onl", "online", "ooo", "open", "org", "organic", "orgcn", "osaka", "ott", "ovh", "pa", "page", "paris", "pars", "parts", "party", "pay", "pccw", "pe", "pet", "pet", "pf", "pg", "ph", "phd", "phone", "photo", "pics", "pid", "pin", "ping", "pink", "pink", "pizza", "pk", "pl", "place", "play", "plus", "plus", "pm", "pn", "pnc", "pohl", "poker", "poker", "porn", "post", "pr", "praxi", "press", "press", "prime", "pro", "pro", "prod", "prof", "promo", "promo", "protection", "pru", "ps", "pt", "pub", "pub", "pw", "pw", "pwc", "py", "qa", "qhcn", "qpon", "quest", "quest", "radio", "re", "read", "red", "red", "rehab", "reise", "reit", "ren", "ren", "rent", "rent", "rest", "rich", "ricoh", "ril", "rio", "rip", "ro", "rocks", "rodeo", "room", "rs", "rsvp", "ru", "rugby", "ruhr", "run", "run", "rw", "rwe", "sa", "safe", "sale", "sale", "salon", "sap", "sarl", "sas", "save", "saxo", "saxo", "sb", "sbi", "sbs", "sc", "sca", "scb", "sccn", "school", "scot", "sd", "sdcn", "se", "seat", "security", "seek", "sener", "ses", "seven", "sew", "sex", "sexy", "sfr", "sg", "sh", "sharp", "shaw", "shcn", "shell", "shia", "shoes", "shop", "shop", "shopping", "show", "show", "si", "silk", "sina", "site", "site", "sj", "sk", "ski", "ski", "skin", "skin", "sky", "skype", "sl", "sling", "sm", "smart", "smile", "sn", "sncf", "sncn", "so", "social", "sohu", "sohu", "solar", "song", "sony", "soy", "spa", "space", "space", "sport", "spot", "sr", "srl", "ss", "st", "stada", "star", "stc", "storage", "store", "store", "studio", "study", "su", "surf", "sv", "swiss", "sx", "sxcn", "sy", "sz", "tab", "talk", "tatar", "tax", "tax", "taxi", "tc", "tci", "td", "tdk", "team", "team", "tech", "tech", "technology", "tel", "teva", "tf", "tg", "th", "thd", "theatre", "tiaa", "tickets", "tips", "tires", "tirol", "tj", "tjcn", "tjx", "tk", "tl", "tm", "tmall", "tn", "to", "today", "today", "tokyo", "tools", "top", "top", "toray", "total", "tours", "town", "toys", "tr", "trade", "trust", "trv", "tt", "tube", "tui", "tunes", "tushu", "tv", "tv", "tvs", "tw", "twcn", "tz", "ua", "ubank", "ubs", "ug", "uk", "unicom", "uno", "uno", "uol", "ups", "us", "uy", "uz", "va", "vana", "vc", "ve", "vegas", "vet", "vg", "vi", "video", "video", "vig", "vin", "vin", "vip", "vip", "visa", "viva", "vivo", "vn", "vodka", "volvo", "vote", "vote", "voto", "voto", "vu", "wales", "wang", "wang", "watch", "weber", "website", "weibo", "weir", "wf", "wien", "wiki", "wiki", "win", "wine", "wme", "work", "work", "works", "world", "world", "wow", "ws", "wtc", "wtf", "xbox", "xerox", "xin", "xin", "xjcn", "xxx", "xyz", "xyz", "xzcn", "yachts", "yahoo", "ye", "yncn", "yoga", "yoga", "you", "yt", "yun", "yun", "zara", "zero", "zip", "zjcn", "zm", "zone", "zone", "zw", "ελ", "ευ", "бг", "ею", "рф", "世界", "中信", "中国", "中文网", "企业", "佛山", "信息", "健康", "公司", "公益", "公益cn", "商城", "商店", "商标", "在线", "娱乐", "广东", "我爱你", "手机", "招聘", "政务", "政务cn", "时尚", "游戏", "移动", "网址", "网店", "网站", "网络", "联通", "购物", "集团", "餐厅", "香港"}
	__doubleBlockDomains   = []string{"a.bg", "a.se", "ab.ca", "abo.pa", "ac.ae", "ac.at", "ac.be", "ac.ci", "ac.cn", "ac.cr", "ac.cy", "ac.fj", "ac.gn", "ac.id", "ac.il", "ac.im", "ac.in", "ac.ir", "ac.ke", "ac.kr", "ac.lk", "ac.ls", "ac.ma", "ac.mu", "ac.mw", "ac.mz", "ac.ni", "ac.nz", "ac.pa", "ac.pr", "ac.rs", "ac.ru", "ac.rw", "ac.se", "ac.sz", "ac.th", "ac.tj", "ac.tz", "ac.ug", "ac.uk", "ac.vn", "ac.za", "ac.zm", "ac.zw", "act.au", "adult.ht", "adv.mz", "aero.mv", "agric.za", "agro.bo", "ah.cn", "ai.in", "aip.ee", "aland.fi", "alt.za", "am.in", "app.gp", "art.do", "art.dz", "art.ht", "art.sn", "arte.bo", "arts.co", "arts.nf", "arts.ro", "arts.ve", "asn.au", "asn.lv", "ass.km", "assn.lk", "asso.bj", "asso.ci", "asso.dz", "asso.fr", "asso.gp", "asso.ht", "asso.km", "asso.mc", "asso.nc", "asso.re", "at.md", "at.vg", "ath.cx", "av.tr", "awdev.ca", "b.bg", "b.se", "base.ec", "base.shop", "bbs.tr", "bc.ca", "bd.se", "be.ax", "be.gy", "bel.tr", "belau.pw", "bet.ar", "bib.ve", "bihar.in", "bip.sh", "bir.ru", "biz.at", "biz.az", "biz.bb", "biz.cy", "biz.et", "biz.fj", "biz.gl", "biz.id", "biz.in", "biz.ki", "biz.ls", "biz.mv", "biz.mw", "biz.my", "biz.ni", "biz.nr", "biz.pk", "biz.pr", "biz.ss", "biz.tj", "biz.tr", "biz.ua", "biz.vn", "biz.wf", "biz.zm", "bj.cn", "blog.bo", "blog.gt", "blog.kg", "blog.vu", "bnr.la", "brand.se", "busan.kr", "c.bg", "c.la", "c.se", "ca.in", "ca.na", "caa.li", "carrd.co", "cat.ax", "cbg.ru", "cc.hn", "cc.na", "cc.ua", "cci.fr", "ch.tc", "ck.ua", "cloud.goog", "cn.in", "cn.ua", "cn.vu", "cnpy.gdn", "co.ae", "co.ag", "co.am", "co.ao", "co.at", "co.bb", "co.bi", "co.bn", "co.bw", "co.ca", "co.ci", "co.cl", "co.cm", "co.cr", "co.cz", "co.gl", "co.gy", "co.id", "co.il", "co.im", "co.in", "co.ir", "co.je", "co.ke", "co.kr", "co.krd", "co.lc", "co.ls", "co.ma", "co.mg", "co.mu", "co.mw", "co.mz", "co.na", "co.ni", "co.nl", "co.nz", "co.om", "co.place", "co.pn", "co.pw", "co.ro", "co.rs", "co.rw", "co.st", "co.sz", "co.th", "co.tj", "co.tm", "co.tz", "co.ua", "co.ug", "co.uk", "co.uz", "co.ve", "co.vi", "co.za", "co.zm", "co.zw", "col.ng", "com.ac", "com.af", "com.ag", "com.al", "com.am", "com.ar", "com.au", "com.aw", "com.az", "com.ba", "com.bb", "com.bh", "com.bi", "com.bm", "com.bn", "com.bo", "com.bt", "com.by", "com.bz", "com.ci", "com.cm", "com.cn", "com.co", "com.cu", "com.cv", "com.cw", "com.cy", "com.de", "com.dm", "com.do", "com.dz", "com.ec", "com.ee", "com.eg", "com.es", "com.et", "com.fj", "com.fm", "com.fr", "com.ge", "com.gh", "com.gi", "com.gl", "com.gn", "com.gp", "com.gr", "com.gt", "com.gu", "com.gy", "com.hk", "com.hn", "com.hr", "com.ht", "com.im", "com.in", "com.iq", "com.is", "com.jo", "com.kg", "com.ki", "com.km", "com.kp", "com.kw", "com.ky", "com.kz", "com.la", "com.lb", "com.lc", "com.lk", "com.lr", "com.lv", "com.ly", "com.mg", "com.mk", "com.ml", "com.mo", "com.ms", "com.mt", "com.mu", "com.mv", "com.mw", "com.mx", "com.my", "com.na", "com.nf", "com.ng", "com.ni", "com.nr", "com.om", "com.pa", "com.pe", "com.pf", "com.ph", "com.pk", "com.pr", "com.ps", "com.pt", "com.py", "com.qa", "com.re", "com.ro", "com.ru", "com.sa", "com.sb", "com.sc", "com.sd", "com.se", "com.sg", "com.sh", "com.sl", "com.sn", "com.ss", "com.st", "com.sv", "com.sy", "com.tj", "com.tm", "com.tn", "com.tr", "com.ua", "com.ug", "com.uy", "com.uz", "com.vc", "com.ve", "com.vi", "com.vn", "com.vu", "com.ws", "com.ye", "com.zm", "conf.au", "conf.lv", "conf.se", "conn.uk", "coop.ar", "coop.ht", "coop.in", "coop.km", "coop.mv", "coop.mw", "coop.py", "coop.rw", "copro.uk", "cq.cn", "cr.ua", "crd.co", "cri.nz", "cs.in", "cv.ua", "cx.ua", "d.bg", "d.se", "daegu.kr", "dapps.earth", "ddnss.de", "de.cool", "de.gt", "de.ls", "de.md", "delhi.in", "demon.nl", "desa.id", "dev.vu", "dn.ua", "dp.ua", "dr.in", "dr.na", "dr.tr", "drr.ac", "dy.fi", "e.bg", "e.se", "ed.ao", "ed.ci", "ed.cr", "ed.pw", "edu.ac", "edu.af", "edu.al", "edu.ar", "edu.au", "edu.az", "edu.ba", "edu.bb", "edu.bh", "edu.bi", "edu.bm", "edu.bn", "edu.bo", "edu.bt", "edu.bz", "edu.ci", "edu.cn", "edu.co", "edu.cu", "edu.cv", "edu.cw", "edu.dm", "edu.do", "edu.dz", "edu.ec", "edu.ee", "edu.eg", "edu.es", "edu.et", "edu.fm", "edu.gd", "edu.ge", "edu.gh", "edu.gi", "edu.gl", "edu.gn", "edu.gp", "edu.gr", "edu.gt", "edu.gu", "edu.gy", "edu.hk", "edu.hn", "edu.ht", "edu.in", "edu.iq", "edu.is", "edu.jo", "edu.kg", "edu.ki", "edu.km", "edu.kn", "edu.kp", "edu.krd", "edu.kw", "edu.ky", "edu.kz", "edu.la", "edu.lb", "edu.lc", "edu.lk", "edu.lr", "edu.ls", "edu.lv", "edu.ly", "edu.mg", "edu.mk", "edu.ml", "edu.mn", "edu.mo", "edu.ms", "edu.mt", "edu.mv", "edu.mw", "edu.mx", "edu.my", "edu.mz", "edu.ng", "edu.ni", "edu.nr", "edu.om", "edu.pa", "edu.pe", "edu.pf", "edu.ph", "edu.pk", "edu.pn", "edu.pr", "edu.ps", "edu.pt", "edu.py", "edu.qa", "edu.rs", "edu.ru", "edu.sa", "edu.sb", "edu.sc", "edu.scot", "edu.sd", "edu.sg", "edu.sl", "edu.sn", "edu.ss", "edu.st", "edu.sv", "edu.sy", "edu.tj", "edu.tm", "edu.tr", "edu.ua", "edu.uy", "edu.vc", "edu.ve", "edu.vn", "edu.vu", "edu.ws", "edu.ye", "edu.za", "edu.zm", "emb.kw", "ens.tn", "er.in", "es.ax", "es.kr", "est.pr", "eu.ax", "eu.int", "eu.org", "eun.eg", "exnet.su", "f.bg", "f.se", "fam.pk", "fbxos.fr", "fh.se", "fhsk.se", "fhv.se", "fi.cr", "fie.ee", "fin.ci", "fin.ec", "fin.tn", "firm.co", "firm.ht", "firm.in", "firm.nf", "firm.ng", "firm.ro", "firm.ve", "fj.cn", "flap.id", "forte.id", "free.hr", "from.hr", "g.bg", "g.se", "gc.ca", "gd.cn", "geek.nz", "gen.in", "gen.ng", "gen.nz", "gen.tr", "gg.ax", "go.ci", "go.cr", "go.id", "go.ke", "go.kr", "go.pw", "go.th", "go.tj", "go.tz", "go.ug", "gob.ar", "gob.bo", "gob.cl", "gob.do", "gob.ec", "gob.es", "gob.gt", "gob.hn", "gob.mx", "gob.ni", "gob.pa", "gob.pe", "gob.pk", "gob.sv", "gob.ve", "goip.de", "gok.pk", "gon.pk", "gop.pk", "gos.pk", "gouv.bj", "gouv.ci", "gouv.fr", "gouv.ht", "gouv.km", "gouv.ml", "gouv.sn", "gov.ac", "gov.ae", "gov.af", "gov.al", "gov.ar", "gov.as", "gov.au", "gov.az", "gov.ba", "gov.bb", "gov.bf", "gov.bh", "gov.bm", "gov.bn", "gov.bt", "gov.by", "gov.bz", "gov.cd", "gov.cl", "gov.cm", "gov.cn", "gov.co", "gov.cu", "gov.cx", "gov.cy", "gov.dm", "gov.do", "gov.dz", "gov.ec", "gov.ee", "gov.eg", "gov.et", "gov.fj", "gov.gd", "gov.ge", "gov.gh", "gov.gi", "gov.gn", "gov.gr", "gov.gu", "gov.gy", "gov.hk", "gov.ie", "gov.il", "gov.in", "gov.iq", "gov.ir", "gov.is", "gov.jo", "gov.kg", "gov.ki", "gov.km", "gov.kn", "gov.kp", "gov.kw", "gov.kz", "gov.la", "gov.lb", "gov.lc", "gov.lk", "gov.lr", "gov.ls", "gov.lt", "gov.lv", "gov.ly", "gov.ma", "gov.mg", "gov.mk", "gov.ml", "gov.mn", "gov.mo", "gov.mr", "gov.ms", "gov.mu", "gov.mv", "gov.mw", "gov.my", "gov.mz", "gov.ng", "gov.nl", "gov.nr", "gov.om", "gov.ph", "gov.pk", "gov.pn", "gov.pr", "gov.ps", "gov.pt", "gov.py", "gov.qa", "gov.rs", "gov.ru", "gov.rw", "gov.sa", "gov.sb", "gov.sc", "gov.scot", "gov.sd", "gov.sg", "gov.sh", "gov.sl", "gov.ss", "gov.sx", "gov.sy", "gov.tj", "gov.tl", "gov.tm", "gov.tn", "gov.tr", "gov.ua", "gov.uk", "gov.vc", "gov.ve", "gov.vn", "gov.ws", "gov.ye", "gov.za", "gov.zm", "gov.zw", "govt.nz", "greta.fr", "grp.lk", "gs.cn", "gsj.bz", "guam.gu", "gub.uy", "gv.ao", "gv.at", "gv.vc", "gx.cn", "gz.cn", "h.bg", "h.se", "ha.cn", "hb.cn", "he.cn", "hi.cn", "hk.cn", "hl.cn", "hn.cn", "hosp.uk", "hotel.lk", "hotel.tz", "hs.kr", "hs.zone", "i.bg", "i.ng", "i.ph", "i.se", "id.au", "id.ir", "id.lv", "id.ly", "idf.il", "idv.hk", "if.ua", "iki.fi", "in.na", "in.ni", "in.rs", "in.th", "in.ua", "inc.hk", "ind.gt", "ind.in", "ind.kw", "ind.tn", "indie.porn", "inf.cu", "inf.mk", "inf.ua", "info.at", "info.au", "info.az", "info.bb", "info.bo", "info.co", "info.cx", "info.ec", "info.et", "info.fj", "info.gu", "info.ht", "info.in", "info.ke", "info.ki", "info.la", "info.ls", "info.mv", "info.na", "info.nf", "info.ni", "info.nr", "info.pk", "info.pr", "info.ro", "info.sd", "info.tn", "info.tr", "info.tz", "info.ve", "info.vn", "info.zm", "ing.pa", "int.ar", "int.az", "int.bo", "int.ci", "int.co", "int.cv", "int.in", "int.is", "int.la", "int.lk", "int.mv", "int.mw", "int.ni", "int.pt", "int.ru", "int.tj", "int.ve", "int.vn", "intl.tn", "io.in", "io.kg", "iris.arpa", "isla.pr", "it.ao", "iwi.nz", "iz.hr", "j.bg", "jeju.kr", "jl.cn", "jozi.biz", "jp.kg", "jp.md", "js.cn", "ju.mp", "jx.cn", "k.bg", "k.se", "kapsi.fi", "kep.tr", "kg.kr", "kh.ua", "kiev.ua", "kiwi.nz", "km.ua", "kr.ua", "krym.ua", "ks.ua", "kv.ua", "kyiv.ua", "l.bg", "l.se", "lab.ms", "law.za", "lenug.su", "lg.ua", "lib.ee", "lima.zone", "ln.cn", "lt.ua", "ltd.cy", "ltd.gi", "ltd.hk", "ltd.lk", "ltd.ng", "ltd.ua", "ltd.uk", "lutsk.ua", "lv.ua", "lviv.ua", "m.bg", "m.se", "maori.nz", "mb.ca", "mc.ax", "mcdir.ru", "mcpre.ru", "md.ci", "me.in", "me.ke", "me.ss", "me.tc", "me.tz", "me.uk", "me.vu", "med.ec", "med.ee", "med.ht", "med.ly", "med.om", "med.pa", "med.sa", "med.sd", "mi.th", "mil.ac", "mil.ae", "mil.al", "mil.ar", "mil.az", "mil.ba", "mil.bo", "mil.by", "mil.cl", "mil.cn", "mil.co", "mil.cy", "mil.do", "mil.ec", "mil.eg", "mil.fj", "mil.ge", "mil.gh", "mil.gt", "mil.hn", "mil.id", "mil.in", "mil.iq", "mil.jo", "mil.kg", "mil.km", "mil.kr", "mil.kz", "mil.lv", "mil.mg", "mil.mv", "mil.my", "mil.mz", "mil.ng", "mil.ni", "mil.nz", "mil.pe", "mil.ph", "mil.py", "mil.qa", "mil.ru", "mil.rw", "mil.sh", "mil.st", "mil.sy", "mil.tj", "mil.tm", "mil.tr", "mil.tz", "mil.uy", "mil.vc", "mil.ve", "mil.ye", "mil.za", "mil.zm", "mil.zw", "mine.nu", "mk.ua", "mo.cn", "mobi.gp", "mobi.ke", "mobi.na", "mobi.ng", "mobi.tz", "mod.gi", "ms.kr", "msk.ru", "msk.su", "muni.il", "mx.na", "my.id", "mycd.eu", "myftp.biz", "mypi.co", "mytis.ru", "n.bg", "n.se", "name.az", "name.eg", "name.et", "name.fj", "name.hr", "name.jo", "name.mk", "name.mv", "name.my", "name.na", "name.ng", "name.pm", "name.pr", "name.qa", "name.tj", "name.tr", "name.vn", "nat.tn", "navoi.su", "nb.ca", "nc.tr", "ne.ke", "ne.kr", "ne.pw", "ne.tz", "ne.ug", "neko.am", "net.ac", "net.ae", "net.af", "net.ag", "net.al", "net.am", "net.ar", "net.au", "net.az", "net.ba", "net.bb", "net.bh", "net.bm", "net.bn", "net.bo", "net.bt", "net.bz", "net.ci", "net.cm", "net.cn", "net.co", "net.cu", "net.cw", "net.cy", "net.dm", "net.do", "net.dz", "net.ec", "net.eg", "net.et", "net.fj", "net.fm", "net.ge", "net.gl", "net.gn", "net.gp", "net.gr", "net.gt", "net.gu", "net.gy", "net.hk", "net.hn", "net.ht", "net.id", "net.il", "net.im", "net.in", "net.iq", "net.ir", "net.is", "net.je", "net.jo", "net.kg", "net.ki", "net.kn", "net.kw", "net.ky", "net.kz", "net.la", "net.lb", "net.lc", "net.lk", "net.lr", "net.ls", "net.lv", "net.ly", "net.ma", "net.mk", "net.ml", "net.mo", "net.ms", "net.mt", "net.mu", "net.mv", "net.mw", "net.mx", "net.my", "net.mz", "net.nf", "net.ng", "net.ni", "net.nr", "net.nz", "net.om", "net.pa", "net.pe", "net.ph", "net.pk", "net.pn", "net.pr", "net.ps", "net.pt", "net.py", "net.qa", "net.ru", "net.rw", "net.sa", "net.sb", "net.sc", "net.sd", "net.sg", "net.sh", "net.sl", "net.ss", "net.st", "net.sy", "net.th", "net.tj", "net.tm", "net.tn", "net.tr", "net.ua", "net.uk", "net.uy", "net.uz", "net.vc", "net.ve", "net.vi", "net.vn", "net.vu", "net.ws", "net.ye", "net.za", "net.zm", "nf.ca", "ngo.lk", "ngo.ng", "ngo.ph", "ngo.za", "nhs.uk", "nic.in", "nic.tj", "nic.za", "nis.za", "nl.ca", "nl.ci", "nm.cn", "noho.st", "nom.ad", "nom.ag", "nom.co", "nom.es", "nom.fr", "nom.km", "nom.mg", "nom.nc", "nom.ni", "nom.pa", "nom.pe", "nom.re", "nom.ro", "nom.tm", "nom.ve", "nom.za", "nome.cv", "nome.pt", "nov.ru", "nov.su", "now.sh", "ns.ca", "nsw.au", "nt.au", "nt.ca", "nt.ro", "ntdll.top", "nu.ca", "nx.cn", "nyaa.am", "nyc.mn", "o.bg", "o.se", "od.ua", "odesa.ua", "of.by", "of.je", "og.ao", "omg.lol", "on.ca", "onred.one", "or.at", "or.bi", "or.ci", "or.cr", "or.id", "or.ke", "or.kr", "or.mu", "or.na", "or.pw", "or.th", "or.tz", "or.ug", "org.ac", "org.ae", "org.af", "org.ag", "org.al", "org.am", "org.ar", "org.au", "org.az", "org.ba", "org.bb", "org.bh", "org.bi", "org.bm", "org.bn", "org.bo", "org.bt", "org.bw", "org.bz", "org.ci", "org.cn", "org.co", "org.cu", "org.cv", "org.cw", "org.cy", "org.dm", "org.do", "org.dz", "org.ec", "org.ee", "org.eg", "org.es", "org.et", "org.fj", "org.fm", "org.ge", "org.gh", "org.gi", "org.gl", "org.gn", "org.gp", "org.gr", "org.gt", "org.gu", "org.gy", "org.hk", "org.hn", "org.ht", "org.il", "org.im", "org.in", "org.iq", "org.ir", "org.is", "org.je", "org.jo", "org.kg", "org.ki", "org.km", "org.kn", "org.kp", "org.kw", "org.ky", "org.kz", "org.la", "org.lb", "org.lc", "org.lk", "org.lr", "org.ls", "org.lv", "org.ly", "org.ma", "org.mg", "org.mk", "org.ml", "org.mn", "org.mo", "org.ms", "org.mt", "org.mu", "org.mv", "org.mw", "org.mx", "org.my", "org.mz", "org.na", "org.ng", "org.ni", "org.nr", "org.nz", "org.om", "org.pa", "org.pe", "org.pf", "org.ph", "org.pk", "org.pn", "org.pr", "org.ps", "org.pt", "org.py", "org.qa", "org.ro", "org.rs", "org.ru", "org.rw", "org.sa", "org.sb", "org.sc", "org.sd", "org.se", "org.sg", "org.sh", "org.sl", "org.sn", "org.ss", "org.st", "org.sv", "org.sy", "org.sz", "org.tj", "org.tm", "org.tn", "org.tr", "org.ua", "org.ug", "org.uk", "org.uy", "org.uz", "org.vc", "org.ve", "org.vi", "org.vn", "org.vu", "org.ws", "org.ye", "org.yt", "org.za", "org.zm", "org.zw", "orx.biz", "otap.co", "other.nf", "own.pm", "owo.codes", "ox.rs", "oy.lc", "oz.au", "p.bg", "p.se", "parti.se", "pb.ao", "pe.ca", "pe.kr", "penza.su", "per.la", "per.nf", "per.sg", "perso.ht", "perso.sn", "perso.tn", "pg.in", "pl.ua", "plc.ly", "plc.uk", "plo.ps", "pol.dz", "pol.ht", "pol.tr", "port.fr", "post.in", "pp.az", "pp.ru", "pp.se", "pp.ua", "prd.fr", "prd.km", "prd.mg", "press.cy", "press.ma", "press.se", "pri.ee", "priv.at", "pro.az", "pro.cy", "pro.ec", "pro.fj", "pro.ht", "pro.in", "pro.mv", "pro.na", "pro.om", "pro.pr", "pro.vn", "prof.pr", "pub.sa", "publ.pt", "pvt.ge", "pymnt.uk", "q.bg", "qc.ca", "qh.cn", "qld.au", "r.bg", "r.se", "radio.am", "radio.fm", "rar.ve", "ras.ru", "re.kr", "realm.cz", "rec.co", "rec.nf", "rec.ro", "rec.ve", "red.sv", "rel.ht", "rep.kp", "repl.co", "res.in", "riik.ee", "rivne.ua", "ro.im", "rovno.ua", "rs.ba", "rv.ua", "s.bg", "s.se", "sa.au", "sa.cr", "salud.bo", "sb.ua", "sc.cn", "sc.ke", "sc.kr", "sc.ls", "sc.tz", "sc.ug", "sch.ae", "sch.id", "sch.ir", "sch.jo", "sch.lk", "sch.ly", "sch.ng", "sch.qa", "sch.sa", "sch.ss", "sch.tf", "sch.uk", "sch.wf", "sch.zm", "sci.eg", "sd.cn", "sec.ps", "seoul.kr", "sh.cn", "shop.ht", "shop.ro", "shop.th", "sk.ca", "sld.do", "sld.pa", "sm.ua", "sn.cn", "soc.dz", "soc.lk", "sochi.su", "spb.ru", "spb.su", "spdns.de", "spdns.eu", "store.bb", "store.nf", "store.ro", "store.st", "store.ve", "storj.farm", "sumy.ua", "sx.cn", "t.bg", "t.se", "tas.au", "te.ua", "tec.ve", "tel.tr", "test.ru", "test.tj", "tj.cn", "tksat.bo", "tm.cy", "tm.dz", "tm.fr", "tm.km", "tm.mc", "tm.mg", "tm.ro", "tm.se", "tm.za", "to.gt", "to.md", "tra.kp", "tsk.tr", "tt.im", "tula.su", "tur.ar", "tuva.su", "tv.bb", "tv.bo", "tv.im", "tv.in", "tv.kg", "tv.na", "tv.sd", "tv.tr", "tv.tz", "tw.cn", "u.bg", "u.se", "ua.rs", "uber.space", "uk.in", "uk.kg", "ulsan.kr", "univ.sn", "up.in", "uri.arpa", "urn.arpa", "us.ax", "us.in", "us.kg", "us.na", "user.fm", "uz.ua", "v.bg", "v.ua", "vic.au", "vn.ua", "volyn.ua", "vxl.sh", "w.bg", "w.se", "wa.au", "we.tc", "web.bo", "web.co", "web.do", "web.gu", "web.id", "web.in", "web.lk", "web.nf", "web.ni", "web.pk", "web.tj", "web.tr", "web.ve", "web.za", "wiki.bo", "ws.na", "x.bg", "x.se", "xj.cn", "xx.gl", "xy.ax", "xz.cn", "y.bg", "y.se", "yalta.ua", "yk.ca", "yn.cn", "ynh.fr", "z.bg", "z.se", "za.bz", "zapto.xyz", "zj.cn", "zp.ua", "zt.ua"}
	__blackWordsMainDomain = []string{
		"a", "css", "js", "slice", "prototype", "t", "o", "this", "f", "i", "n", "c", "date", "list",
		"base64", "div", "li", "response",
	}
)

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

func ExtractDomains(code string, tryDecode ...bool) []string {
	results, rootDomains := scan(code, tryDecode...)
	return utils.RemoveRepeatStringSlice(append(rootDomains, results...))
}

func ExtractRootDomains(code string) []string {
	results, rootDomains := scan(code)
	f := filter.NewFilter()
	var ret []string
	for _, i := range append(rootDomains, results...) {
		r := ExtractRootDomain(i)
		if f.Exist(r) {
			continue
		}
		f.Insert(r)
		ret = append(ret, r)
	}
	f.Close()
	return ret
}

func ExtractDomainsEx(code string) ([]string, []string) {
	re1, re2 := scan(code)
	var ret []string
	for _, i := range append(re1, re2...) {
		r := ExtractRootDomain(i)
		if !slices.Contains(ret, r) {
			ret = append(ret, r)
		}
	}
	return re1, ret
}

func scan(code string, tryDecode ...bool) ([]string, []string) {
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
	if len(tryDecode) > 0 && tryDecode[0] {
		code = TryDecode(code)
	}
	return _scan(code)
}

func _scan(code string) ([]string, []string) {
	scanner := bufio.NewScanner(bytes.NewBufferString(code))
	scanner.Split(bufio.ScanBytes)

	var lastCh byte
	var ch byte
	var blockSeries []string
	currentBlock := bytes.NewBufferString("")

	isDomainChar := func(c byte) bool {
		return (c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			c == '-'
	}
	var results []string
	var rootDomains []string
	resultsFilter := filter.NewFilter()
	defer resultsFilter.Close()
	rootDomainFilter := filter.NewFilter()
	defer rootDomainFilter.Close()
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

		validDomainChar := isDomainChar(ch)

		if ch == '.' && currentBlock.Len() > 0 {
			// block end
			if currentBlock.Len() < 4096 {
				blockSeries = append(blockSeries, currentBlock.String())
			}
			currentBlock.Reset()
			continue
		}

		if validDomainChar {
			// currentBlock += fmt.Sprintf("%c", ch)
			currentBlock.WriteByte(ch)
			continue
		}

		// block end
		if ret := currentBlock.Len(); ret < 4096 && ret > 0 {
			blockSeries = append(blockSeries, currentBlock.String())
		}
		currentBlock.Reset()

		_, ret := haveDomainSuffix(blockSeries)
		if len(blockSeries) > 1 && ret {
			addResult(strings.Join(blockSeries, "."))
		}
		blockSeries = nil
	}
	if ret := currentBlock.Len(); ret > 0 && ret < 4096 {
		blockSeries = append(blockSeries, currentBlock.String())
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
