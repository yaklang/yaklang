module github.com/yaklang/yaklang

go 1.20

replace github.com/yaklang/yaklang v0.0.0 => ./

require (
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49
	github.com/PuerkitoBio/goquery v1.6.0
	github.com/ReneKroon/ttlcache v1.6.0
	github.com/StackExchange/wmi v1.2.1
	github.com/akutz/memconn v0.1.0
	github.com/andybalholm/brotli v1.0.4
	github.com/antchfx/htmlquery v1.2.5
	github.com/antchfx/xmlquery v1.2.3
	github.com/antchfx/xpath v1.2.1
	github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20220911224424-aa1f1f12a846
	github.com/apex/log v1.9.0
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/atotto/clipboard v0.1.2
	github.com/bcicen/jstream v0.0.0-20190220045926-16c1f8af81c2
	github.com/buger/jsonparser v1.1.1
	github.com/c-bata/go-prompt v0.2.3
	github.com/corpix/uarand v0.2.0
	github.com/dave/jennifer v1.4.1
	github.com/davecgh/go-spew v1.1.1
	github.com/denisbrodbeck/machineid v1.0.1
	github.com/denisenkom/go-mssqldb v0.0.0-20190412130859-3b1d194e553a
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/dlclark/regexp2 v1.2.0
	github.com/docker/docker v1.13.1
	github.com/docker/go-connections v0.4.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/fxsjy/RF.go v0.0.0-20140710024358-46700521f302
	github.com/gilliek/go-opml v1.0.0
	github.com/glaslos/ssdeep v0.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/go-ldap/ldap v3.0.3+incompatible
	github.com/go-pg/pg/v10 v10.9.1
	github.com/go-redis/redis/v8 v8.8.2
	github.com/go-resty/resty/v2 v2.7.0
	github.com/go-rod/rod v0.108.1
	github.com/go-sql-driver/mysql v1.5.0
	github.com/gobwas/glob v0.2.3
	github.com/gobwas/httphead v0.1.0
	github.com/gocolly/colly v1.2.0
	github.com/gofrs/uuid v4.0.0+incompatible
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4
	github.com/google/go-github/v33 v33.0.0
	github.com/google/gopacket v1.1.19
	github.com/google/gxui v0.0.0-20151028112939-f85e0a97b3a4
	github.com/google/shlex v0.0.0-20181106134648-c34317bd91bf
	github.com/google/uuid v1.3.0
	github.com/googollee/go-socket.io v1.6.1
	github.com/gorilla/mux v1.7.4
	github.com/gorilla/websocket v1.4.2
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/go-version v1.6.0
	github.com/hpcloud/tail v1.0.0
	github.com/huin/asn1ber v0.0.0-20120622192748-af09f62e6358
	github.com/icodeface/grdp v0.0.0-20200414055757-e0008b0b5cb2
	github.com/icodeface/tls v0.0.0-20190904083142-17aec93c60e5
	github.com/itchyny/gojq v0.12.8
	github.com/jinzhu/copier v0.0.0-20190625015134-976e0346caa8
	github.com/jinzhu/gorm v1.9.2
	github.com/jlaffaye/ftp v0.0.0-20210307004419-5d4190119067
	github.com/k0kubun/pp v3.0.1+incompatible
	github.com/kataras/golog v0.0.10
	github.com/kataras/pio v0.0.2
	github.com/kevinburke/ssh_config v0.0.0-20201106050909-4977a11b4351
	github.com/lestrrat/go-file-rotatelogs v0.0.0-20180223000712-d3151e2a480f
	github.com/lor00x/goldap v0.0.0-20180618054307-a546dffdd1a3
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40
	github.com/mattn/go-sqlite3 v1.10.0
	github.com/mdlayher/arp v0.0.0-20191213142603-f72070a231fc
	github.com/mfonda/simhash v0.0.0-20151007195837-79f94a1100d6
	github.com/miekg/dns v1.1.50
	github.com/mitchellh/go-vnc v0.0.0-20150629162542-723ed9867aed
	github.com/mitchellh/mapstructure v1.4.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oschwald/maxminddb-golang v1.7.0
	github.com/paulmach/go.geojson v1.4.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/sftp v1.11.0
	github.com/projectdiscovery/retryabledns v1.0.13
	github.com/rocket049/gocui v0.3.2
	github.com/saintfish/chardet v0.0.0-20120816061221-3af4cd4741ca
	github.com/satori/go.uuid v1.2.0
	github.com/segmentio/ksuid v1.0.4
	github.com/shirou/gopsutil/v3 v3.22.6
	github.com/shirou/w32 v0.0.0-20160930032740-bb4de0191aa4
	github.com/stacktitan/smb v0.0.0-20190531122847-da9a425dceb8
	github.com/streadway/amqp v0.0.0-20190827072141-edfb9018d271
	github.com/stretchr/testify v1.8.0
	github.com/tatsushid/go-fastping v0.0.0-20160109021039-d7bb493dee3e
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/tidwall/gjson v1.14.4
	github.com/twmb/murmur3 v1.1.6
	github.com/urfave/cli v1.22.1
	github.com/valyala/bytebufferpool v1.0.0
	github.com/vjeantet/grok v1.0.0
	github.com/xiecat/wsm v0.1.3
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292
	golang.org/x/net v0.2.0
	golang.org/x/oauth2 v0.0.0-20220411215720-9780585627b5
	golang.org/x/sys v0.6.0
	golang.org/x/text v0.4.0
	google.golang.org/grpc v1.44.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/fatih/set.v0 v0.2.1
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/sourcemap.v1 v1.0.5
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	moul.io/http2curl v1.0.0
	rsc.io/qr v0.2.0
)

require (
	cloud.google.com/go v0.65.0 // indirect
	github.com/Microsoft/go-winio v0.4.16 // indirect
	github.com/Mzack9999/go-http-digest-auth-client v0.6.1-0.20220414142836-eb8883508809 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20210428141323-04723f9f07d7 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/aliyun/aliyun-oss-go-sdk v2.2.7+incompatible // indirect
	github.com/andybalholm/cascadia v1.1.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0-20190314233015-f79a8a8ca69d // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/emirpasic/gods v1.12.0 // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/fastly/go-utils v0.0.0-20180712184237-d95a45783239 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-gl/gl v0.0.0-20211210172815-726fda9656d6 // indirect
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20200222043503-6f7a984d4dc4 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-pg/zerochecker v0.2.0 // indirect
	github.com/gogs/chardet v0.0.0-20211120154057-b7413eaefb8f // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/gomodule/redigo v1.8.4 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/gopherjs/gopherjs v0.0.0-20181017120253-0766667cb4d1 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/goxjs/gl v0.0.0-20210104184919-e3fafc6f8f2a // indirect
	github.com/goxjs/glfw v0.0.0-20191126052801-d2efb5f20838 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/itchyny/timefmt-go v0.1.3 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jehiah/go-strftime v0.0.0-20171201141054-1d33003b3869 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.0.0 // indirect
	github.com/k0kubun/colorstring v0.0.0-20150214042306-9440f1994b88 // indirect
	github.com/kennygrant/sanitize v1.2.4 // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/lestrrat/go-envload v0.0.0-20180220120943-6ed08b54a570 // indirect
	github.com/lestrrat/go-strftime v0.0.0-20180220042222-ba3bf9c1d042 // indirect
	github.com/lib/pq v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mattn/go-tty v0.0.0-20190424173100-523744f04859 // indirect
	github.com/mdlayher/ethernet v0.0.0-20190313224307-5b5fc417d966 // indirect
	github.com/mdlayher/raw v0.0.0-20190313224157-43dbcdd7739d // indirect
	github.com/microcosm-cc/bluemonday v1.0.19 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/nsf/termbox-go v0.0.0-20191229070316-58d4fcbce2a7 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/term v0.0.0-20190109203006-aa71e9d9e942 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/projectdiscovery/blackrock v0.0.0-20210415162320-b38689ae3a2e // indirect
	github.com/projectdiscovery/fileutil v0.0.0-20220705195237-01becc2a8963 // indirect
	github.com/projectdiscovery/iputil v0.0.0-20220620153941-036d511e4097 // indirect
	github.com/projectdiscovery/mapcidr v1.0.0 // indirect
	github.com/projectdiscovery/retryablehttp-go v1.0.3-0.20220506110515-811d938bd26d // indirect
	github.com/projectdiscovery/stringsutil v0.0.0-20220612082425-0037ce9f89f3 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/tebeka/strftime v0.1.3 // indirect
	github.com/temoto/robotstxt v1.1.1 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/vmihailenco/bufpool v0.1.11 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.0 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xanzy/ssh-agent v0.3.0 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/ysmood/gson v0.7.1 // indirect
	github.com/ysmood/leakless v0.8.0 // indirect
	github.com/yuin/charsetutil v1.0.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opentelemetry.io/otel v0.19.0 // indirect
	go.opentelemetry.io/otel/metric v0.19.0 // indirect
	go.opentelemetry.io/otel/trace v0.19.0 // indirect
	go.uber.org/goleak v1.1.11 // indirect
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e // indirect
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210624195500-8bfb893ecb84 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	honnef.co/go/js/dom v0.0.0-20210725211120-f030747120f2 // indirect
	mellium.im/sasl v0.2.1 // indirect
)
