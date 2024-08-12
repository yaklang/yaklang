package java

import (
	"fmt"
	"testing"
)

func Test_HTTP_SSRF(t *testing.T) {
	t.Run("aTaintCase023", func(t *testing.T) {
		tt := &TestCase{
			Name: "aTaintCase023",
			Code: `
                /**
                    * 字符级别
                    * case应该被检出
                    * @param url
                    * @return
                */
                @PostMapping(value = "case023")
                public Map<String, Object> aTaintCase023(@RequestParam String path) {
                    Map<String, Object> modelMap = new HashMap<>();
                    HttpUtil httpUtil = new HttpUtil();
                    try {
                        httpUtil.doGet(path+"/api/test.json");
                        modelMap.put("status", "success");
                    } catch (Exception e) {
                        modelMap.put("status", "error");
                    }
                    return modelMap;
                }
                `,
			Contain: true,
			Expect: map[string][]string{
				"target": {
					"Parameter-param", "Parameter-url", "Parameter-url", "Undefined-HttpClients", "Undefined-HttpClients", "Undefined-HttpGet", "Undefined-HttpGet", "Undefined-HttpPost", "Undefined-HttpPost", "Undefined-URIBuilder", "Undefined-URIBuilder", "neq(Parameter-param, nil)", "nil",
				},
			},
		}
		tt.Code = createHttpUtilCode(tt.Code)
		testRequestTopDef(t, tt)
	})

	t.Run("aTaintCase023", func(t *testing.T) {
		tt := &TestCase{
			Name: "aTaintCase023_2",
			Code: `
                @PostMapping(value = "case023")
                public Map<String, Object> aTaintCase023(@RequestParam String path) {
                    Map<String, Object> modelMap = new HashMap<>();
                    HttpUtil httpUtil = new HttpUtil();
                    try {
                        httpUtil.doGet("/api/test.json");
                        modelMap.put("status", "success");
                    } catch (Exception e) {
                        modelMap.put("status", "error");
                    }
                    return modelMap;
                }
                `,
			Contain: true,
			Expect: map[string][]string{
				"target": {
					"Parameter-param", "Parameter-url", "Parameter-url", "Undefined-HttpClients", "Undefined-HttpClients", "Undefined-HttpGet", "Undefined-HttpGet", "Undefined-HttpPost", "Undefined-HttpPost", "Undefined-URIBuilder", "Undefined-URIBuilder", "neq(Parameter-param, nil)", "nil",
				}},
		}
		tt.Code = createHttpUtilCode(tt.Code)
		testRequestTopDef(t, tt)
	})
}

func createHttpUtilCode(code string) string {
	allCode := fmt.Sprintf(`
package com.sast.astbenchmark.common.utils;

public class HttpUtil {

    public String doGet(String url, Map<String, String> param) throws Exception {

        // 创建Httpclient对象
        CloseableHttpClient httpclient = HttpClients.createDefault();

        String resultString = "";
        CloseableHttpResponse response = null;
        try {
            // 创建uri
            URIBuilder builder = new URIBuilder(url);
            if (param != null) {
                for (String key : param.keySet()) {
                    builder.addParameter(key, param.get(key));
                }
            }
            URI uri = builder.build();

            // 创建http GET请求
            HttpGet httpGet = new HttpGet(uri);
            RequestConfig requestConfig = RequestConfig.custom().setConnectTimeout(10000).setConnectionRequestTimeout(10000).setSocketTimeout(10000).build();
            httpGet.setConfig(requestConfig);
            // 执行请求
            response = httpclient.execute(httpGet);
            // 判断返回状态是否为200
            if (response.getStatusLine().getStatusCode() == 200) {
                resultString = EntityUtils.toString(response.getEntity(), "UTF-8");
            }
        } catch (Exception e) {
            throw e;
        } finally {
            try {
                if (response != null) {
                    response.close();
                }
                httpclient.close();
            } catch (Exception e) {
                throw e;
            }
        }
        return resultString;
    }

    public static String doGet(String url) throws Exception {
        return doGet(url, null);
    }

    public static String doPost(String url, Map<String, String> param) throws Exception {
        // 创建Httpclient对象
        CloseableHttpClient httpClient = HttpClients.createDefault();
        CloseableHttpResponse response = null;
        String resultString = "";
        try {
            // 创建Http Post请求
            HttpPost httpPost = new HttpPost(url);
            RequestConfig requestConfig = RequestConfig.custom().setConnectTimeout(10000).setConnectionRequestTimeout(10000).setSocketTimeout(10000).build();
            httpPost.setConfig(requestConfig);
            // 创建参数列表
            if (param != null) {
                List<NameValuePair> paramList = new ArrayList<>();
                for (String key : param.keySet()) {
                    paramList.add(new BasicNameValuePair(key, param.get(key)));
                }
                // 模拟表单
                UrlEncodedFormEntity entity = new UrlEncodedFormEntity(paramList, "utf-8");
                httpPost.setEntity(entity);
            }
            // 执行http请求
            response = httpClient.execute(httpPost);
            resultString = EntityUtils.toString(response.getEntity(), "utf-8");
        } catch (Exception e) {
            throw e;
        } finally {
            try {
                response.close();
            } catch (Exception e) {
                throw e;
            }
        }

        return resultString;
    }

    public static String doPost(String url) throws Exception {
        return doPost(url, null);
    }


}
public class AstTaintCase002 {
%v
}

`, code)
	return allCode
}
