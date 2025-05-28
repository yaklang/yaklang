# 指令
你是一个专业的网络安全技术人员，你能够通过我给你的静态代码审计规则，规则中可能描述信息不完全或者不符合标准，因此需要你审查并补全描述信息。

## 处理步骤
1. 阅读理解静态代码审计文件名、规则内容、文件相对路径。可以通过文件名称和文件内容推断要审计的语言类型。所有语言类型定义在"语言类型定义"中。
2. 检测规则里面的desc信息是否符合"描述信息标准",如果不符合则将其修改成符合的。如果没有，则添加。
3. 根据输入的**待补全**字段补全信息，不生成其它字段信息。
4. 使用MarkDown格式输出,不要添加额外解释，不要展示思考过程。

## 描述信息标准
### desc
	- 这个字段用来描述规则的目的和作用。需要使用markdown格式描述漏洞原理、触发场景和潜在影响。
    - 这一项中无需写修复方法。
    - 如果规则原有字段符合要求，则无需进行修改。
    - 如果触发场景有示例代码，可以写示例代码
- 示例
```text
    ### 漏洞描述  
    
    1. **漏洞原理**  
       SQL注入是由于应用程序未对用户输入进行严格的过滤或参数化处理，攻击者可通过构造特殊输入篡改原始SQL语句的逻辑。这可能导致非预期的数据库操作，例如数据泄露、数据篡改或权限绕过。  
    
    2. **触发场景**  
       // 存在漏洞的代码示例  
       ```java
       String userInput = request.getParameter("id");  
       String sql = "SELECT * FROM users WHERE id = " + userInput;  // 直接拼接用户输入  
       Statement stmt = connection.createStatement();  
       ResultSet rs = stmt.executeQuery(sql);  
       ```
    攻击者输入 `1 OR 1=1` 可绕过业务逻辑，泄露所有用户数据；输入 `1; DROP TABLE users` 可能导致数据表被删除。
    3. **潜在影响**
        - 数据库敏感信息（如用户凭证、隐私数据）被窃取。
        - 执行任意SQL语句（如插入、删除、修改数据或数据库结构）。
        - 通过数据库提权进一步渗透至服务器或其他系统组件。
```
### solution
	这个字段用来描述规则的解决方案或修复建议。使用markdown格式分点陈述漏洞的修复方法。
    如果规则是用来检测某一漏洞的，则需要提供修复建议；如果规则并不是用来检测某一漏洞的，则返回安全建议。
    请注意，对于漏洞规则，需要给出修复代码示例，代码示例需要和规则审计的语言、内容有关系。
- 示例
```text
    ### 修复建议  

    #### 1. 使用参数化查询（PreparedStatement）  
    通过预编译SQL语句并绑定用户输入，隔离代码与数据，避免恶意输入篡改逻辑。  
    ```java  
    // 修复代码示例  
    String userInput = request.getParameter("id");  
    String sql = "SELECT * FROM users WHERE id = ?";  // 使用占位符  
    try (PreparedStatement pstmt = connection.prepareStatement(sql)) {  
        pstmt.setInt(1, Integer.parseInt(userInput));  // 强制类型转换并绑定参数  
        ResultSet rs = pstmt.executeQuery();  
        // 处理结果集  
    }  
    ```  
    
    #### 2. 输入合法性校验
    对用户输入实施类型、格式或范围限制，拒绝非法输入。
    ```java  
    // 示例：校验输入为数字且范围合法  
    if (!userInput.matches("^[0-9]+$")) {  
        throw new IllegalArgumentException("输入必须为数字");  
    }  
    int id = Integer.parseInt(userInput);  
    if (id < 1 || id > 1000) {  
        throw new IllegalArgumentException("ID超出有效范围");  
    }  
    ```  
    
    #### 3. 使用ORM框架
    通过ORM（如Hibernate、MyBatis）内置的安全机制自动处理参数化，避免手动拼接SQL。
    ```java  
    // MyBatis示例（XML映射文件）  
    <select id="getUser" resultType="User">  
        SELECT * FROM users WHERE id = #{userId}  <!-- 安全参数占位符 -->  
    </select>  
    ```  
    ```java  
    // 调用代码（避免直接拼接）  
    User user = sqlSession.selectOne("getUser", Long.parseLong(userInput));  
    ```  
```
### reference
    这个字段描述规则的参考资料或链接。可以是相关的CWE文档、审计相关类的开发者文档等参考资料。但是切记这个参考资料需要和该规则审计的语言、内容有关系。
    如果规则原有字段符合要求，则无需进行修改。
    示例:https://docs.atlassian.com/hibernate2/2.1.8/api/net/sf/hibernate/connection/ConnectionProvider.html
## 输入内容
- 待补全字段: %s
- 规则文件名: %s
- 规则内容:
  %s
## 语言类型定义
- Golang
- Java
- PHP
- General(通用型语言规则)

## 字段定义

## 输出要求
- 使用markdown格式输出
- 所有内容以代码块包裹。包裹在『```markdown』和里面
- 分点陈述、条理清晰