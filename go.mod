module github.com/yaklang/yaklang

go 1.22.12

replace github.com/yaklang/yaklang v0.0.0 => ./

replace github.com/wenlng/go-captcha-assets v1.0.5 => github.com/wenlng/go-captcha-assets v1.0.4

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/CycloneDX/cyclonedx-go v0.7.2
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49
	github.com/PuerkitoBio/goquery v1.6.0
	github.com/aliyun/aliyun-oss-go-sdk v2.2.7+incompatible
	github.com/andybalholm/brotli v1.0.6
	github.com/antchfx/xmlquery v1.3.1
	github.com/antchfx/xpath v1.3.2
	github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20220911224424-aa1f1f12a846
	github.com/asaskevich/govalidator v0.0.0-20210307081110-f21760c49a8d
	github.com/aymanbagabas/go-pty v0.2.2
	github.com/bcicen/jstream v0.0.0-20190220045926-16c1f8af81c2
	github.com/beevik/etree v1.5.0
	github.com/bytedance/mockey v1.2.14
	github.com/chzyer/readline v1.5.1
	github.com/cloudflare/circl v1.3.7
	github.com/corpix/uarand v0.2.0
	github.com/creack/pty v1.1.21
	github.com/dave/jennifer v1.4.1
	github.com/davecgh/go-spew v1.1.1
	github.com/deckarep/golang-set/v2 v2.7.0
	github.com/denisbrodbeck/machineid v1.0.1
	github.com/denisenkom/go-mssqldb v0.12.3
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13
	github.com/dlclark/regexp2 v1.11.0
	github.com/docker/docker v25.0.6+incompatible
	github.com/docker/go-connections v0.4.0
	github.com/dop251/goja v0.0.0-20240220182346-e401ed450204
	github.com/dop251/goja_nodejs v0.0.0-20240221231712-27eeffc9c235
	github.com/emersion/go-message v0.18.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/fxsjy/RF.go v0.0.0-20140710024358-46700521f302
	github.com/gilliek/go-opml v1.0.0
	github.com/go-git/go-billy/v5 v5.6.0
	github.com/go-git/go-git/v5 v5.13.0
	github.com/go-ldap/ldap v3.0.3+incompatible
	github.com/go-pg/pg/v10 v10.14.0
	github.com/go-rod/rod v0.112.9
	github.com/go-sql-driver/mysql v1.5.0
	github.com/go-viper/mapstructure/v2 v2.2.1
	github.com/gobwas/glob v0.2.3
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/golang/protobuf v1.5.4
	github.com/golang/snappy v0.0.4
	github.com/google/btree v1.0.0
	github.com/google/go-containerregistry v0.15.2
	github.com/google/go-dap v0.10.0
	github.com/google/pprof v0.0.0-20230926050212-f7f687d19a98
	github.com/google/uuid v1.6.0
	github.com/gopacket/gopacket v1.3.1
	github.com/gorilla/mux v1.7.4
	github.com/gorilla/websocket v1.4.2
	github.com/gosnmp/gosnmp v1.35.0
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/go-version v1.6.0
	github.com/hpcloud/tail v1.0.0
	github.com/huin/asn1ber v0.0.0-20120622192748-af09f62e6358
	github.com/icza/bitio v1.1.0
	github.com/itchyny/gojq v0.12.8
	github.com/jellydator/ttlcache/v3 v3.3.0
	github.com/jinzhu/copier v0.0.0-20190625015134-976e0346caa8
	github.com/jinzhu/gorm v1.9.2
	github.com/jlaffaye/ftp v0.0.0-20210307004419-5d4190119067
	github.com/kataras/golog v0.0.10
	github.com/kataras/pio v0.0.2
	github.com/kevinburke/ssh_config v1.2.0
	github.com/klauspost/compress v1.17.4
	github.com/knqyf263/go-rpmdb v0.1.0
	github.com/lestrrat/go-file-rotatelogs v0.0.0-20180223000712-d3151e2a480f
	github.com/liamg/jfather v0.0.7
	github.com/lor00x/goldap v0.0.0-20180618054307-a546dffdd1a3
	github.com/lunixbochs/struc v0.0.0-20200707160740-784aaebc1d40
	github.com/mailru/easyjson v0.9.0
	github.com/mattn/go-sqlite3 v1.14.15
	github.com/mdlayher/arp v0.0.0-20191213142603-f72070a231fc
	github.com/mdlayher/netlink v1.1.1
	github.com/mfonda/simhash v0.0.0-20151007195837-79f94a1100d6
	github.com/miekg/dns v1.1.50
	github.com/mitchellh/go-vnc v0.0.0-20150629162542-723ed9867aed
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826
	github.com/olekukonko/tablewriter v0.0.5
	github.com/oschwald/maxminddb-golang v1.7.0
	github.com/paulmach/go.geojson v1.4.0
	github.com/pkg/errors v0.9.1
	github.com/pkg/sftp v1.11.0
	github.com/projectdiscovery/gostruct v0.0.0-20230520110439-bbdedaae3c35
	github.com/quic-go/quic-go v0.48.1
	github.com/rabbitmq/amqp091-go v1.9.0
	github.com/refraction-networking/utls v1.6.7
	github.com/saintfish/chardet v0.0.0-20120816061221-3af4cd4741ca
	github.com/samber/lo v1.38.1
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1
	github.com/segmentio/ksuid v1.0.4
	github.com/shirou/gopsutil/v3 v3.23.4
	github.com/sijms/go-ora/v2 v2.7.19
	github.com/stacktitan/smb v0.0.0-20190531122847-da9a425dceb8
	github.com/steambap/captcha v1.4.1
	github.com/stretchr/testify v1.10.0
	github.com/tatsushid/go-fastping v0.0.0-20160109021039-d7bb493dee3e
	github.com/tevino/abool v0.0.0-20170917061928-9b9efcf221b5
	github.com/tidwall/gjson v1.14.4
	github.com/tidwall/sjson v1.2.5
	github.com/twmb/murmur3 v1.1.6
	github.com/urfave/cli v1.22.15
	github.com/valyala/bytebufferpool v1.0.0
	github.com/vjeantet/grok v1.0.0
	github.com/xdg-go/pbkdf2 v1.0.0
	github.com/xdg-go/scram v1.1.2
	github.com/xdg-go/stringprep v1.0.4
	github.com/xuri/excelize/v2 v2.9.0
	github.com/yaklang/fastgocaptcha v1.0.4
	github.com/yaklang/pcap v1.0.5
	github.com/ysmood/gson v0.7.3
	github.com/ysmood/leakless v0.8.0
	go.mongodb.org/mongo-driver v1.12.1
	go.uber.org/atomic v1.7.0
	golang.org/x/crypto v0.33.0
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56
	golang.org/x/image v0.18.0
	golang.org/x/mod v0.19.0
	golang.org/x/net v0.35.0
	golang.org/x/sys v0.30.0
	golang.org/x/term v0.29.0
	golang.org/x/text v0.22.0
	golang.org/x/time v0.5.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	google.golang.org/grpc v1.69.4
	google.golang.org/protobuf v1.36.6
	gopkg.in/fatih/set.v0 v0.2.1
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	gotest.tools/v3 v3.5.0
	rsc.io/qr v0.2.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v1.1.3 // indirect
	github.com/andybalholm/cascadia v1.1.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.14.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/cyphar/filepath-securejoin v0.2.5 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dop251/base64dec v0.0.0-20231022112746-c6c9f9a96217 // indirect
	github.com/emersion/go-textwrapper v0.0.0-20200911093747-65d896831594 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/fastly/go-utils v0.0.0-20180712184237-d95a45783239 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-pg/zerochecker v0.2.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-sql/civil v0.0.0-20190719163853-cb61b32ac6fe // indirect
	github.com/golang-sql/sqlexp v0.1.0 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/gopherjs/gopherjs v1.12.80 // indirect
	github.com/itchyny/timefmt-go v0.1.3 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jehiah/go-strftime v0.0.0-20171201141054-1d33003b3869 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.0.0 // indirect
	github.com/jonboulle/clockwork v0.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/lestrrat/go-envload v0.0.0-20180220120943-6ed08b54a570 // indirect
	github.com/lestrrat/go-strftime v0.0.0-20180220042222-ba3bf9c1d042 // indirect
	github.com/lib/pq v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mdlayher/ethernet v0.0.0-20190606142754-0394541c37b7 // indirect
	github.com/mdlayher/raw v0.0.0-20191009151244-50f2db8cc065 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/montanaflynn/stats v0.0.0-20171201202039-1bf9dbcd8cbe // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/ginkgo/v2 v2.9.5 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc3 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/shoenig/go-m1cpu v0.1.5 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/smartystreets/assertions v1.2.0 // indirect
	github.com/smartystreets/goconvey v1.7.2 // indirect
	github.com/tebeka/strftime v0.1.3 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/tmthrgd/go-hex v0.0.0-20190904060850-447a3041c3bc // indirect
	github.com/u-root/u-root v0.11.0 // indirect
	github.com/vbatts/tar-split v0.11.3 // indirect
	github.com/vmihailenco/bufpool v0.1.11 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.4 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wenlng/go-captcha-assets v1.0.5 // indirect
	github.com/wenlng/go-captcha/v2 v2.0.3 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	github.com/xuri/efp v0.0.0-20240408161823-9ad904a10d6d // indirect
	github.com/xuri/nfp v0.0.0-20240318013403-ab9948c2c4a7 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	github.com/ysmood/fetchup v0.2.2 // indirect
	github.com/ysmood/goob v0.4.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.47.0 // indirect
	go.opentelemetry.io/otel v1.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.22.0 // indirect
	go.opentelemetry.io/otel/metric v1.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.34.0 // indirect
	go.uber.org/mock v0.4.0 // indirect
	golang.org/x/arch v0.11.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/tools v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241015192408-796eee8c2d53 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	mellium.im/sasl v0.3.1 // indirect
)
