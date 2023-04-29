package quakeschema

import "time"

type QuakeResult struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []Data `json:"data"`
	Meta    Meta   `json:"meta"`
}
type CookieElement struct {
	OrderHash string `json:"order_hash"`
	Simhash   string `json:"simhash"`
	Key       string `json:"key"`
}
type DomTree struct {
	DomHash string `json:"dom_hash"`
	Key     string `json:"key"`
	Simhash string `json:"simhash"`
}
type Favicon struct {
	Data     string `json:"data"`
	Location string `json:"location"`
	Hash     string `json:"hash"`
}
type CSS struct {
	Class []interface{} `json:"class"`
	ID    []interface{} `json:"id"`
}
type Script struct {
	Variable []interface{} `json:"variable"`
	Function []interface{} `json:"function"`
}
type DomElement struct {
	CSS    CSS    `json:"css"`
	Script Script `json:"script"`
}
type Link struct {
	Iframe []interface{} `json:"iframe"`
	Script []interface{} `json:"script"`
	Other  []interface{} `json:"other"`
	Img    []interface{} `json:"img"`
}
type Information struct {
	PhoneNum  []interface{} `json:"phone_num"`
	Icp       string        `json:"icp"`
	Copyright []interface{} `json:"copyright"`
	Mail      []interface{} `json:"mail"`
}
type HTTP struct {
	Server          string        `json:"server"`
	StatusCode      int           `json:"status_code"`
	CookieElement   CookieElement `json:"cookie_element"`
	ResponseHeaders string        `json:"response_headers"`
	HTMLHash        string        `json:"html_hash"`
	DomTree         DomTree       `json:"dom_tree"`
	Favicon         Favicon       `json:"favicon"`
	Body            string        `json:"body"`
	MetaElement     []interface{} `json:"meta_element"`
	RobotsHash      string        `json:"robots_hash"`
	XPoweredBy      string        `json:"x_powered_by"`
	SitemapHash     string        `json:"sitemap_hash"`
	DomElement      DomElement    `json:"dom_element"`
	Link            Link          `json:"link"`
	HTTPLoadCount   int           `json:"http_load_count"`
	Title           string        `json:"title"`
	MetaKeywords    string        `json:"meta_keywords"`
	Host            string        `json:"host"`
	Information     Information   `json:"information"`
	HeaderOrderHash string        `json:"header_order_hash"`
	PageType        []interface{} `json:"page_type"`
	Path            string        `json:"path"`
	HTTPLoadURL     []string      `json:"http_load_url"`
	Robots          string        `json:"robots"`
	SecurityText    string        `json:"security_text"`
	Sitemap         string        `json:"sitemap"`
}

type Location struct {
	StreetCn    string    `json:"street_cn"`
	Radius      float64   `json:"radius"`
	Isp         string    `json:"isp"`
	Gps         []float64 `json:"gps"`
	CountryEn   string    `json:"country_en"`
	StreetEn    string    `json:"street_en"`
	CityCn      string    `json:"city_cn"`
	ProvinceCn  string    `json:"province_cn"`
	CityEn      string    `json:"city_en"`
	ProvinceEn  string    `json:"province_en"`
	CountryCode string    `json:"country_code"`
	DistrictEn  string    `json:"district_en"`
	Owner       string    `json:"owner"`
	DistrictCn  string    `json:"district_cn"`
	CountryCn   string    `json:"country_cn"`
}
type Components struct {
	ProductType    []string `json:"product_type"`
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	ProductNameCn  string   `json:"product_name_cn"`
	ProductNameEn  string   `json:"product_name_en"`
	ProductLevel   string   `json:"product_level"`
	ProductCatalog []string `json:"product_catalog"`
	ProductVendor  string   `json:"product_vendor"`
}
type ClientFinished struct {
	VerifyData string `json:"verify_data"`
}
type CipherSuites struct {
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type CompressionMethods struct {
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type SignatureAndHashes struct {
	HashAlgorithm      string `json:"hash_algorithm"`
	SignatureAlgorithm string `json:"signature_algorithm"`
}
type SupportedCurves struct {
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type SupportedPointFormats struct {
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type Version struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type ClientHello struct {
	AlpnProtocols           []string                `json:"alpn_protocols"`
	CipherSuites            []CipherSuites          `json:"cipher_suites"`
	CompressionMethods      []CompressionMethods    `json:"compression_methods"`
	ExtendedMasterSecret    bool                    `json:"extended_master_secret"`
	Heartbeat               bool                    `json:"heartbeat"`
	NextProtocolNegotiation bool                    `json:"next_protocol_negotiation"`
	OcspStapling            bool                    `json:"ocsp_stapling"`
	Random                  string                  `json:"random"`
	SctEnabled              bool                    `json:"sct_enabled"`
	Scts                    bool                    `json:"scts"`
	SecureRenegotiation     bool                    `json:"secure_renegotiation"`
	ServerName              string                  `json:"server_name"`
	SignatureAndHashes      []SignatureAndHashes    `json:"signature_and_hashes"`
	SupportedCurves         []SupportedCurves       `json:"supported_curves"`
	SupportedPointFormats   []SupportedPointFormats `json:"supported_point_formats"`
	Ticket                  bool                    `json:"ticket"`
	Version                 Version                 `json:"version"`
}
type ClientPrivate struct {
	Length int    `json:"length"`
	Value  string `json:"value"`
}
type X struct {
	Length int    `json:"length"`
	Value  string `json:"value"`
}
type Y struct {
	Length int    `json:"length"`
	Value  string `json:"value"`
}
type ClientPublic struct {
	X X `json:"x"`
	Y Y `json:"y"`
}
type CurveID struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
type EcdhParams struct {
	ClientPrivate ClientPrivate `json:"client_private"`
	ClientPublic  ClientPublic  `json:"client_public"`
	CurveID       CurveID       `json:"curve_id"`
	ServerPublic  ServerPublic  `json:"server_public"`
}
type ClientKeyExchange struct {
	EcdhParams EcdhParams `json:"ecdh_params"`
}
type MasterSecret struct {
	Length int    `json:"length"`
	Value  string `json:"value"`
}
type PreMasterSecret struct {
	Length int    `json:"length"`
	Value  string `json:"value"`
}
type KeyMaterial struct {
	MasterSecret    MasterSecret    `json:"master_secret"`
	PreMasterSecret PreMasterSecret `json:"pre_master_secret"`
}
type BasicConstraints struct {
	IsCa bool `json:"is_ca"`
}
type Extensions struct {
	AuthorityKeyID   string           `json:"authority_key_id"`
	BasicConstraints BasicConstraints `json:"basic_constraints"`
	SubjectKeyID     string           `json:"subject_key_id"`
}
type Issuer struct {
	CommonName []string `json:"common_name"`
}
type SignatureAlgorithm struct {
	Name string `json:"name"`
	Oid  string `json:"oid"`
}

type Subject struct {
	CommonName []string `json:"common_name"`
}
type KeyAlgorithm struct {
	Name string `json:"name"`
}
type RsaPublicKey struct {
	Exponent int    `json:"exponent"`
	Length   int    `json:"length"`
	Modulus  string `json:"modulus"`
}
type SubjectKeyInfo struct {
	FingerprintSha256 string       `json:"fingerprint_sha256"`
	KeyAlgorithm      KeyAlgorithm `json:"key_algorithm"`
	RsaPublicKey      RsaPublicKey `json:"rsa_public_key"`
}
type Validity struct {
	End    time.Time `json:"end"`
	Length int       `json:"length"`
	Start  time.Time `json:"start"`
}
type Parsed struct {
	Extensions             Extensions         `json:"extensions"`
	FingerprintMd5         string             `json:"fingerprint_md5"`
	FingerprintSha1        string             `json:"fingerprint_sha1"`
	FingerprintSha256      string             `json:"fingerprint_sha256"`
	Issuer                 Issuer             `json:"issuer"`
	IssuerDn               string             `json:"issuer_dn"`
	Names                  []string           `json:"names"`
	Redacted               bool               `json:"redacted"`
	SerialNumber           string             `json:"serial_number"`
	Signature              Signature          `json:"signature"`
	SignatureAlgorithm     SignatureAlgorithm `json:"signature_algorithm"`
	SpkiSubjectFingerprint string             `json:"spki_subject_fingerprint"`
	Subject                Subject            `json:"subject"`
	SubjectDn              string             `json:"subject_dn"`
	SubjectKeyInfo         SubjectKeyInfo     `json:"subject_key_info"`
	TbsFingerprint         string             `json:"tbs_fingerprint"`
	TbsNoctFingerprint     string             `json:"tbs_noct_fingerprint"`
	ValidationLevel        string             `json:"validation_level"`
	Validity               Validity           `json:"validity"`
	Version                int                `json:"version"`
}
type Certificate struct {
	Parsed Parsed `json:"parsed"`
	Raw    string `json:"raw"`
}
type Validation struct {
	BrowserError   string `json:"browser_error"`
	BrowserTrusted bool   `json:"browser_trusted"`
}
type ServerCertificates struct {
	Certificate Certificate `json:"certificate"`
	Validation  Validation  `json:"validation"`
}
type ServerFinished struct {
	VerifyData string `json:"verify_data"`
}
type CipherSuite struct {
	Hex   string `json:"hex"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type ServerHello struct {
	AlpnProtocol         string      `json:"alpn_protocol"`
	CipherSuite          CipherSuite `json:"cipher_suite"`
	CompressionMethod    int         `json:"compression_method"`
	ExtendedMasterSecret bool        `json:"extended_master_secret"`
	Heartbeat            bool        `json:"heartbeat"`
	OcspStapling         bool        `json:"ocsp_stapling"`
	Random               string      `json:"random"`
	SecureRenegotiation  bool        `json:"secure_renegotiation"`
	SessionID            string      `json:"session_id"`
	Ticket               bool        `json:"ticket"`
	Version              Version     `json:"version"`
}
type ServerPublic struct {
	X X `json:"x"`
	Y Y `json:"y"`
}

type SignatureAndHashType struct {
	HashAlgorithm      string `json:"hash_algorithm"`
	SignatureAlgorithm string `json:"signature_algorithm"`
}
type TLSVersion struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}
type Signature struct {
	Raw                  string               `json:"raw"`
	HashAlgorithm        string               `json:"hash_algorithm"`
	SelfSigned           bool                 `json:"self_signed"`
	SignatureAlgorithm   SignatureAlgorithm   `json:"signature_algorithm"`
	SignatureAndHashType SignatureAndHashType `json:"signature_and_hash_type"`
	TLSVersion           TLSVersion           `json:"tls_version"`
	Type                 string               `json:"type"`
	Valid                bool                 `json:"valid"`
	Value                string               `json:"value"`
}
type ServerKeyExchange struct {
	Digest     string     `json:"digest"`
	EcdhParams EcdhParams `json:"ecdh_params"`
	Signature  Signature  `json:"signature"`
}
type HandshakeLog struct {
	ClientFinished     ClientFinished     `json:"client_finished"`
	ClientHello        ClientHello        `json:"client_hello"`
	ClientKeyExchange  ClientKeyExchange  `json:"client_key_exchange"`
	KeyMaterial        KeyMaterial        `json:"key_material"`
	ServerCertificates ServerCertificates `json:"server_certificates"`
	ServerFinished     ServerFinished     `json:"server_finished"`
	ServerHello        ServerHello        `json:"server_hello"`
	ServerKeyExchange  ServerKeyExchange  `json:"server_key_exchange"`
}
type TLS struct {
	HandshakeLog HandshakeLog `json:"handshake_log"`
}
type TLSJarm struct {
	JarmAns  []string `json:"jarm_ans"`
	JarmHash string   `json:"jarm_hash"`
}
type Service struct {
	HTTP     HTTP    `json:"http"`
	Banner   string  `json:"banner"`
	Name     string  `json:"name"`
	Product  string  `json:"product"`
	Response string  `json:"response"`
	TLS      TLS     `json:"tls"`
	TLSJarm  TLSJarm `json:"tls-jarm"`
	Cert     string  `json:"cert"`
	Version  string  `json:"version"`
}
type Data struct {
	Service    Service       `json:"service,omitempty"`
	Org        string        `json:"org"`
	Location   Location      `json:"location"`
	Time       time.Time     `json:"time"`
	Components []Components  `json:"components"`
	OsName     string        `json:"os_name"`
	Images     []interface{} `json:"images"`
	Hostname   string        `json:"hostname"`
	Port       int           `json:"port"`
	IsIpv6     bool          `json:"is_ipv6"`
	IP         string        `json:"ip"`
	Transport  string        `json:"transport"`
	Asn        int           `json:"asn"`
	OsVersion  string        `json:"os_version"`
	Domain     string        `json:"domain,omitempty"`
}
type Meta struct {
	Total        int64  `json:"total"`
	PaginationID string `json:"pagination_id"`
}
