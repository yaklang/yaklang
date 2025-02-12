package yakit

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type PortsTypeGroup struct {
	Nginx                   int32
	Apache                  int32
	IIS                     int32
	Litespeed               int32
	Tomcat                  int32
	ApacheTrafficServer     int32
	OracleHTTPServer        int32
	Openresty               int32
	Jetty                   int32
	Caddy                   int32
	Gunicorn                int32
	Cowboy                  int32
	Lighttpd                int32
	Resin                   int32
	Zeus                    int32
	Cherrypy                int32
	Tengine                 int32
	Glassfish               int32
	PhusionPassenger        int32
	Tornadoserver           int32
	Hiawatha                int32
	OracleApplicationServer int32
	AbyssWebServer          int32
	Boa                     int32
	Xitami                  int32
	Simplehttp              int32
	Cherokee                int32
	MonkeyHTTPServer        int32
	NodeJS                  int32
	Websphere               int32
	Zope                    int32
	Mongoose                int32
	Macos                   int32
	Kestrel                 int32
	Aolserver               int32
	Dnsmasq                 int32
	Ruby                    int32
	Webrick                 int32
	WeblogicServer          int32
	Jboss                   int32
	SqlServer               int32
	Mysql                   int32
	Mongodb                 int32
	Redis                   int32
	Elasticsearch           int32
	Postgresql              int32
	DB2                     int32
	Hbase                   int32
	Memcached               int32
	Splunkd                 int32
}

func CreateOrUpdatePort(db *gorm.DB, hash string, i interface{}) error {
	db = db.Model(&schema.Port{})

	switch ret := i.(type) {
	case *schema.Port:
		var existed schema.Port
		db.Where("hash = ?", hash).First(&existed)
		if existed.ID > 0 {
			p := &existed
			p.HtmlTitle = utils.PrettifyShrinkJoin("|", p.HtmlTitle, ret.HtmlTitle)
			p.ServiceType = utils.PrettifyShrinkJoin("/", p.ServiceType, ret.ServiceType)
			p.CPE = utils.PrettifyShrinkJoin("|", p.CPE, p.CPE)
			p.State = ret.State
			return db.Save(p).Error
		}
	}
	if db := db.Where("hash = ?", hash).Assign(i).FirstOrCreate(&schema.Port{}); db.Error != nil {
		return utils.Errorf("create/update Port failed: %s", db.Error)
	}
	return nil
}

func GetPort(db *gorm.DB, id int64) (*schema.Port, error) {
	var req schema.Port
	if db := db.Model(&schema.Port{}).Where("id = ?", id).First(&req); db.Error != nil {
		return nil, utils.Errorf("get Port failed: %s", db.Error)
	}

	return &req, nil
}

func DeletePortByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.Port{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.Port{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func QueryPorts(db *gorm.DB, params *ypb.QueryPortsRequest) (*bizhelper.Paginator, []*schema.Port, error) {
	if params == nil {
		return nil, nil, utils.Errorf("empty params")
	}
	db = db.Model(&schema.Port{}) // .Debug(
	db = db.Select(`id,created_at,updated_at,cpe,host,port,proto,service_type,task_name,html_title,` + "`from`" + `,hash,state,ip_integer,
--when fingerprint length <=20kb return self
case when 
	length(fingerprint) <= 20480 then fingerprint
--else return substring 
else
	substr(fingerprint, 1, 20480) 
end as fingerprint`)
	if params.Pagination == nil {
		params.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}
	if params.GetAfterUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", params.GetAfterUpdatedAt(), time.Now().Add(10*time.Minute).Unix())
	}
	if params.GetBeforeUpdatedAt() > 0 {
		db = bizhelper.QueryByTimeRangeWithTimestamp(db, "updated_at", 0, params.GetBeforeUpdatedAt())
	}
	if params.GetAfterId() > 0 {
		db = db.Where("id > ?", params.GetAfterId())
	}
	if params.GetBeforeId() > 0 {
		db = db.Where("id < ?", params.GetBeforeId())
	}
	p := params.Pagination
	db = bizhelper.QueryOrder(db, p.OrderBy, p.Order)
	db = FilterPort(db, params)
	/*db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())

	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}*/

	var ret []*schema.Port
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &ret)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}

	return paging, ret, nil
}

func FilterPort(db *gorm.DB, params *ypb.QueryPortsRequest) *gorm.DB {
	db = db.Model(&schema.Port{})
	db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	//db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "host", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"service_type"}, utils.PrettifyListFromStringSplited(params.GetComplexSelect(), ","), false)
	db = bizhelper.FuzzSearchEx(db, []string{
		"port", "host", "service_type", "html_title",
	}, params.GetKeywords(), false)
	if params.GetTitleEffective() {
		db = db.Where("html_title NOT LIKE '%404%' AND html_title <> '' ")
	}
	db = bizhelper.FuzzQueryLike(db, "proto", params.GetProto())
	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}
	if params.GetRuntimeId() != "" {
		db = db.Where("runtime_id = ?", params.GetRuntimeId())
	}
	//else {
	//	db = db.Where("runtime_id is null OR (runtime_id = '')")
	//}
	return db
}

type SimplePort struct {
	Host string
	Port int
}

func YieldSimplePorts(db *gorm.DB, ctx context.Context) chan *SimplePort {
	outC := make(chan *SimplePort)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*SimplePort
			if _, b := bizhelper.NewPagination(&bizhelper.Param{
				DB:    db,
				Page:  page,
				Limit: 1000,
			}, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 1000 {
				return
			}
		}
	}()
	return outC
}

func YieldPorts(db *gorm.DB, ctx context.Context) chan *schema.Port {
	db = db.Model(schema.Port{})
	return bizhelper.YieldModel[*schema.Port](ctx, db)
}

/*func FilterByQueryPorts(db *gorm.DB, params *ypb.QueryPortsRequest) (_ *gorm.DB, _ error) {
	db = db.Model(&Port{})
	db = bizhelper.QueryBySpecificPorts(db, "port", params.GetPorts())
	db = bizhelper.QueryBySpecificAddress(db, "ip_integer", params.GetHosts())
	db = bizhelper.FuzzQueryLike(db, "service_type", params.GetService())
	db = bizhelper.FuzzQueryLike(db, "html_title", params.GetTitle())

	if params.GetState() == "" {
		db = bizhelper.ExactQueryString(db, "state", "open")
	} else {
		db = bizhelper.ExactQueryString(db, "state", params.GetState())
	}
	return db, nil
}*/

func DeletePortsByID(db *gorm.DB, id int64) error {
	if db := db.Model(&schema.Port{}).Where(
		"id = ?", id,
	).Unscoped().Delete(&schema.Port{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func PortsServiceTypeGroup() ([]*PortsTypeGroup, error) {
	var db = consts.GetGormProjectDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}
	var result []*PortsTypeGroup
	db = db.Raw(`
		SELECT
		  SUM(CASE WHEN service_type LIKE '%nginx%' THEN 1 ELSE 0 END) as nginx,
		  SUM(CASE WHEN service_type LIKE '%apache%' THEN 1 ELSE 0 END) AS apache,
		  SUM(CASE WHEN service_type LIKE '%iis%' THEN 1 ELSE 0 END) AS iis,
		  SUM(CASE WHEN service_type LIKE '%litespeed%' THEN 1 ELSE 0 END) AS litespeed,
		  SUM(CASE WHEN service_type LIKE '%tomcat%' THEN 1 ELSE 0 END) AS tomcat,
		  SUM(CASE WHEN service_type LIKE '%oracle_http_server%' THEN 1 ELSE 0 END) AS oracle_http_server,
		  SUM(CASE WHEN service_type LIKE '%openresty%' THEN 1 ELSE 0 END) AS openresty,
		  SUM(CASE WHEN service_type LIKE '%jetty%' THEN 1 ELSE 0 END) AS jetty,
		  SUM(CASE WHEN service_type LIKE '%caddy%' THEN 1 ELSE 0 END) AS caddy,
		  SUM(CASE WHEN service_type LIKE '%gunicorn%' THEN 1 ELSE 0 END) AS gunicorn,
		  SUM(CASE WHEN service_type LIKE '%cowboy%' THEN 1 ELSE 0 END) AS cowboy,
		  SUM(CASE WHEN service_type LIKE '%lighttpd%' THEN 1 ELSE 0 END) AS lighttpd,
		  SUM(CASE WHEN service_type LIKE '%resin%' THEN 1 ELSE 0 END) AS resin,
		  SUM(CASE WHEN service_type LIKE '%zeus%' THEN 1 ELSE 0 END) AS zeus,
		  SUM(CASE WHEN service_type LIKE '%cherrypy%' THEN 1 ELSE 0 END) AS cherrypy,
		  SUM(CASE WHEN service_type LIKE '%tengine%' THEN 1 ELSE 0 END) AS tengine,
		  SUM(CASE WHEN service_type LIKE '%glassfish%' THEN 1 ELSE 0 END) AS glassfish,
		  SUM(CASE WHEN service_type LIKE '%phusion_passenger%' THEN 1 ELSE 0 END) AS phusion_passenger,
		  SUM(CASE WHEN service_type LIKE '%tornadoserver%' THEN 1 ELSE 0 END) AS tornadoserver,
		  SUM(CASE WHEN service_type LIKE '%hiawatha%' THEN 1 ELSE 0 END) AS hiawatha,
		  SUM(CASE WHEN service_type LIKE '%oracle_application_server%' THEN 1 ELSE 0 END) AS oracle_application_server,
		  SUM(CASE WHEN service_type LIKE '%abyss_web_server%' THEN 1 ELSE 0 END) AS abyss_web_server,
		  SUM(CASE WHEN service_type LIKE '%boa%' THEN 1 ELSE 0 END) AS boa,
		  SUM(CASE WHEN service_type LIKE '%xitami%' THEN 1 ELSE 0 END) AS xitami,
		  SUM(CASE WHEN service_type LIKE '%simplehttp%' THEN 1 ELSE 0 END) AS simplehttp,
		  SUM(CASE WHEN service_type LIKE '%cherokee%' THEN 1 ELSE 0 END) AS cherokee,
		  SUM(CASE WHEN service_type LIKE '%monkey_http_server%' THEN 1 ELSE 0 END) AS monkey_http_server,
		  SUM(CASE WHEN service_type LIKE '%node.js%' THEN 1 ELSE 0 END) AS  'node.js',
		  SUM(CASE WHEN service_type like '%websphere%' THEN 1 ELSE 0 END) AS websphere,
		  SUM(CASE WHEN service_type like '%zope%' THEN 1 ELSE 0 END) AS zope,
		  SUM(CASE WHEN service_type like '%mongoose%' THEN 1 ELSE 0 END) AS mongoose,
		  SUM(CASE WHEN service_type like '%macos%' THEN 1 ELSE 0 END) AS   macos,
		  SUM(CASE WHEN service_type like '%kestrel%' THEN 1 ELSE 0 END) AS kestrel ,
		  SUM(CASE WHEN service_type like '%aolserver%' THEN 1 ELSE 0 END) AS  aolserver,
		  SUM(CASE WHEN service_type like '%dnsmasq%' THEN 1 ELSE 0 END) AS dnsmasq,
		  SUM(CASE WHEN service_type like '%ruby%' THEN 1 ELSE 0 END) AS  ruby,
		  SUM(CASE WHEN service_type like '%webrick%' THEN 1 ELSE 0 END) AS  webrick,
		  SUM(CASE WHEN service_type like '%weblogic_server%' THEN 1 ELSE 0 END) AS weblogic_server,
		  SUM(CASE WHEN service_type like '%jboss%' THEN 1 ELSE 0 END) AS  jboss,
		  SUM(CASE WHEN service_type like '%sql_server%' THEN 1 ELSE 0 END) AS sql_server,
		  SUM(CASE WHEN service_type like '%mysql%' THEN 1 ELSE 0 END) AS  mysql,
		  SUM(CASE WHEN service_type like '%mongodb%' THEN 1 ELSE 0 END) AS mongodb,
		  SUM(CASE WHEN service_type like '%redis%' THEN 1 ELSE 0 END) AS redis,
		  SUM(CASE WHEN service_type like '%elasticsearch%' THEN 1 ELSE 0 END) AS elasticsearch,
		  SUM(CASE WHEN service_type like '%postgresql%' THEN 1 ELSE 0 END) AS postgresql,
		  SUM(CASE WHEN service_type like '%db2%' THEN 1 ELSE 0 END) AS  db2,
		  SUM(CASE WHEN service_type like '%hbase%' THEN 1 ELSE 0 END) AS hbase,
		  SUM(CASE WHEN service_type like '%memcached%' THEN 1 ELSE 0 END) AS memcached,
		  SUM(CASE WHEN service_type like '%splunkd%' THEN 1 ELSE 0 END) AS splunkd
		FROM
		  ports;
	`)

	db = db.Scan(&result)
	if db.Error != nil {
		return nil, utils.Errorf("PortsServiceTypeGroup failed: %s", db.Error)
	}

	return result, nil
}
