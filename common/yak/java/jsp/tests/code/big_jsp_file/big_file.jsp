<%@ page language="java" contentType="text/html; charset=gbk" pageEncoding="gbk"%>
<%@ page import="com.simp.view.tag.*"%>
<%@ page import="com.simp.action.RTUser"%>
<%@ page import="com.simp.biz.policy.Policy"%>
<%@ page import="java.util.Set"%>
<%@ page import="java.util.Map"%>
<%@ page import="java.util.LinkedHashMap"%>
<%@ page import="com.simp.util.txt.StrTool"%>
<%@ page import="com.simp.util.jsp.JspTool"%>
<%@ page import="java.util.List"%>
<%@ page import="com.simp.action.portal.ssobean.*"%>
<%@ page import="com.simp.biz.person.CnfAuth"%>
<%@ page import="com.simp.biz.res.Res"%>
<%@ page import="com.simp.biz.res.ResCase"%>
<%@ page import="com.simp.biz.auth.AuthCase"%>
<%@ page import="com.simp.util.jsp.Parameter"%>
<%@ page import="com.simp.action.portal.sso.SSOAuthBean"%>
<!-- yw	2017年3月14日	HA资源单点登录	添加了HA资源单点登录页面 -->
<html>
<head>
<title>HA资源单点登录配置页面</title>
<OBJECT ID="sso" WIDTH="0" HEIGHT="0" CLASSID="CLSID:0A9CBDE6-ED41-418E-9559-4352B6DF6398">
</OBJECT>
<OBJECT ID="sso_0" WIDTH="0" HEIGHT="0" CLASSID="CLSID:24633CC3-641F-46E8-844E-52545262BEF2" ></OBJECT>
<script type="text/javascript" src="/pub/js5/public.js"></script>
<script type="text/javascript" src="/pub/js5/stretch.js"></script>
<script type="text/javascript" src="/pub/js5/XMLDoc.js"></script>
<script type="text/javascript" src="/pub/js5/validate.js"></script>
<script type="text/javascript" src="/pub/js5/tag.js"></script>
<script type="text/javascript" src="/pub/js5/portal/sso.js"></script>
<script type="text/javascript" src="/pub/js5/jquery/jquery.min.js"></script>
<link rel="stylesheet" type="text/css" href="/pub/css5/icon/font-awesome.min.css" />
<link rel="stylesheet" type="text/css" href="/pub/css5/icon/ionicon.min.css"/>
<link rel="stylesheet" type="text/css" href="/pub/css5/common.css"/>
<link rel="stylesheet" type="text/css" href="/pub/css5/portal/table.css" />
<!-- 处理密码中包含特殊字符，引入base64.js -->
<script type="text/javascript" src="/pub/js5/base64.js"></script>
<jsp:include page="/inc/pop_view.jsp"></jsp:include>

<%
	Tag tag = new Tag(request);

	String name = Parameter.WCharStr(request, "name");

	SSOAuthBean bean = new SSOAuthBean(request);
	
	String msg = "";
	CnfAuth auth = null;
	String crt_mode_disabled = "";
	
	if (!bean.isOk()) {
		msg = bean.getReply();
	} else {
		auth = bean.getAuth();
		
		if ("putty".equals(auth.getLogin_tool())) {
			crt_mode_disabled = "disabled";
		}
	}
	
	String workId = bean.getWorkId();
	String is_save = auth.getIs_save();
	
	boolean no_driver = false;
	boolean no_clip = false;
	boolean no_console = false;
	boolean ischk = false;
	if (bean.getWorkCnf() != null) {
		no_clip = "1".equals(bean.getWorkCnf().getClip());
		no_driver = "1".equals(bean.getWorkCnf().getDrivers());
		no_console = "1".equals(bean.getWorkCnf().getIs_console());
	}
%>

<% if(msg.length()>0){ %>
	<script type="text/javascript">		
		var msg = "<%=msg%>";
		alert(msg);
	</script>
<% 
	response.sendRedirect("/pub/search_error.jsp");
	return;
} %>
</head>
<body>
	<div class="right_list_all">
    	<%-- <h4><i class="ion-navigate fa-lg"></i>
    	<% if (workId != null && workId.length() > 0) { %>
    		<small>工单 · 登录审批 · </small>UNIX资源单点登录
    	<% } else { %>
    		<small>单点登录 · <span><%=name%> · </span></small>HA资源单点登录
    	<% } %>
    	</h4> --%>
        <div class="panel">
      	   <div class="panel_body">
           		<div class="table_filter">
                    <form name=fm method=post style="margin:0px">
                        <table id="jb_tb">
                                <tr>
                                    <td width=150 align="right" style=" padding-top:20px;">资源类型：</td>
                                    <td width=20 align="right" style=" padding-top:20px;"></td>
                                    <td id="ptype" width=300 style=" padding-top:20px;">
                                        <input value="<%=ResCase.res_type_des_map().get(auth.getPtype()) %>" disabled type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">资源名称：</td>
                                    <td width=20 align="right"></td>
                                    <td id="pname" width=300>
                                        <input value="<%=auth.getPname() %>" disabled type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">资源IP：</td>
                                    <td width=20 align="right"></td>
                                    <td id="pip" width=300>
                                        <input value="<%=auth.getPip() %>" disabled type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">账号名称：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <input id=acc_name name="acc_name" value="<%=auth.getAcc_name() %>" <%=auth.is$User() ? "" : "readonly" %> type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <% if (auth.is$User()) { %>
                                <tr>
                                    <td width=150 align="right">口令：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <input id=acc_pwd name="acc_pwd" value="<%=auth.getAcc_pwd()!=null&&auth.getAcc_pwd().length()>0?auth.getAcc_pwd():"" %>" type="password" style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <% } %>
                                
                                
                                
                                <% if (ResCase.TYPE_VM_UNIX.equals(auth.getPtype())){ %>
                                
                                
                                <tr>
                                    <td width=150 align="right">登录方式：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                    <div class="sel">
                                    	<select id=sso_type name=sso_type style="" onChange="chg_sso_type();chg_view();">
                                            <!-- 根据访问方式  显示option -->
                                            <%=tag.options_v(bean.getSso_type_map(), bean.getAuth().getSso_type()) %>
										</select>
                                     </div>
                                    </td>
                                    <td>&nbsp;
                                    </td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">协议：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <!-- 字符协议 -->
                                        <% 
                                        String df_protocol = auth.getProtocol();
                                        String df_port = null;
                                        ischk = false;
                                        
                                        if (AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_SSH2)) {
                                            String chk_tag = "";
                                            
                                            if (df_protocol == null || df_protocol.length() == 0 || ResCase.PROTO_SSH2.equals(df_protocol)) {
                                                ischk = true;
                                                chk_tag = "checked";
                                                df_port = auth.getSsh_port();
                                            }
                                            
                                            if (df_port == null || df_port.length() == 0) {
                                                String ssh_port = auth.getSsh_port();
                                                df_port =  (ssh_port == null || ssh_port.length() == 0) ? ResCase.DF_PORT_SSH : ssh_port;
                                            }
                                        %>
                                            <div id="ssh2" style="float: left; padding-left: 10px;"><input style="height:15px; padding-right:10px;" type="radio" name="protocol" <%=chk_tag %> value="<%=ResCase.PROTO_SSH2%>" onClick="chg_port('<%=auth.getRes_rdn() %>','<%=ResCase.PROTO_SSH2 %>')"/><span>SSH2</span></div>
                                        <% } %>
                                        
                                        <% if (AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_SSH1)) {
                                            String chk_tag = ischk ? "" : "checked";
                                            if (df_protocol != null && df_protocol.length() > 0 && df_protocol.equals(ResCase.PROTO_SSH1)) {
                                            	ischk = true;
                                            	chk_tag = "checked";
                                                df_port = auth.getSsh_port();
                                            }
                                            
                                            if (df_port == null || df_port.length() == 0) {
                                                String ssh_port = auth.getSsh_port();
                                                df_port =  (ssh_port == null || ssh_port.length() == 0) ? ResCase.DF_PORT_SSH : ssh_port;
                                            }
                                        %>
                                            <div id="ssh1" style="float: left; padding-left: 10px;"><input style="height:15px; padding-right:5px;" type="radio" name="protocol" <%=chk_tag %> value="<%=ResCase.PROTO_SSH1%>" onClick="chg_port('<%=auth.getRes_rdn() %>','<%=ResCase.PROTO_SSH1 %>')"/><span>SSH1</span></div>
                                        <% } %>
                                        <% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_TELNET)) { 
                                            String chk_tag = ischk ? "" : "checked";
                                            if (df_protocol != null && df_protocol.length() > 0 && df_protocol.equals(ResCase.PROTO_TELNET)) {
                                            	ischk = true;
                                            	chk_tag = "checked";
                                                df_port = auth.getTelnet_port();
                                            }
                                            
                                            if (df_port == null || df_port.length() == 0) {
                                                String telnet_port = auth.getTelnet_port();
                                                df_port =  (telnet_port == null || telnet_port.length() == 0) ? ResCase.DF_PORT_TELNET : telnet_port;
                                            }
                                        %>
                                            <div id="telnet" style="float: left; padding-left: 10px;"><input style="height:15px; padding-right:5px;" type="radio" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_TELNET%>" onClick="chg_port('<%=auth.getRes_rdn() %>','<%=ResCase.PROTO_TELNET %>')" /><span>TELNET</span></div>
                                        <% } %>
                                        
                                        <!-- 图形协议 -->
					    				<% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_VNC)) {
					    					String chk_tag = ischk ? "" : "checked";
					    					if (df_protocol != null && df_protocol.length() > 0 && ResCase.PROTO_VNC.equals(df_protocol)) {
					    						ischk = true;
					    						chk_tag = "checked";
					    						df_port = auth.getVnc_port();
					    					}
					    					
					    					if (df_port == null || df_port.length() == 0) {
					    						String vnc_port = auth.getVnc_port();
				    							df_port = (vnc_port == null || vnc_port.length() == 0) ? ResCase.DF_PORT_VNC : vnc_port;
				    						}							    					
					    				%>
					    					<div id="vnc" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:5px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_VNC%>" onclick="chg_port('<%=auth.getRes_rdn() %>','<%=ResCase.PROTO_VNC %>');"/><span>VNC</span></div>
					    				<% } %>
					    				
					    				<% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_XWIN)) { 
					    					String chk_tag = ischk ? "" : "checked";
				    						if (df_protocol != null && df_protocol.length() > 0 && ResCase.PROTO_XWIN.equals(df_protocol)) {
				    							ischk = true;
				    							chk_tag = "checked";
					    						df_port = auth.getXwin_port();
					    					}
				    						
					    					if (df_port == null || df_port.length() == 0) {
					    						String xwin_port = auth.getXwin_port();
				    							df_port = (xwin_port == null || xwin_port.length() == 0) ? ResCase.DF_PORT_XWIN : xwin_port;
				    						}							    					
					    				%>
					    					<div id="xwin" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:5px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_XWIN%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_XWIN %>');"/><span>XWindow</span></div>
					    				<% } %>
					    				
					    				<!-- xftp -->
                                        <% 	if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_FTP)) {
											String chk_tag = "";
											if (df_protocol == null || df_protocol.length() == 0 || ResCase.PROTO_FTP.equals(df_protocol)) {
												ischk = true;
												chk_tag = "checked";
												df_port = auth.getFtp_port();
											}
											
											if (df_port == null || df_port.length() == 0) {
												String ftp_port = auth.getFtp_port();
												df_port = (ftp_port == null || ftp_port.length() == 0) ? ResCase.DF_PORT_FTP : ftp_port;
											}
										%>
					    					<div id="ftp" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:5px;" name="protocol" <%=chk_tag %> value="<%=ResCase.PROTO_FTP%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_FTP%>')"/><span>FTP</span></div>
					    				<% } %>
					    				<% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_SFTP)) { 
					    					String chk_tag = ischk ? "" : "checked";
					    					if (df_protocol != null && df_protocol.length() > 0 && ResCase.PROTO_SFTP.equals(df_protocol)) {
					    						ischk = true;
					    						chk_tag = "checked";
					    						df_port = auth.getSftp_port();
					    					}
					    					
					    					if (df_port == null || df_port.length() == 0) {
					    						String sftp_port = auth.getSftp_port();
					    						df_port = (sftp_port == null || sftp_port.length() == 0) ? ResCase.DF_PORT_SFTP : sftp_port;
				    						}			
					    				%>
					    					<div id="sftp" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:5px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_SFTP%>" onClick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_SFTP%>')"/><span>SFTP</span></div>
					    				<% } %>
					    				
                                    </td>
                                    <td>&nbsp;
                                        
                                    </td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">端口：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <input id="port" name="port" value="<%=df_port%>" type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
                                <tr id=crt_mode_tr>
                                    <td width=150 align="right">登录形式：</td>
                                    <td width=20 align="right"></td>
                                    <td id=crt_mode1 width=300>
                                    	<div class="sel">
                                            <select id=crt_mode name=crt_mode <%=crt_mode_disabled %>>
                                                <option value="single" <%="single".equals(auth.getCrt_mode()) ? "selected" : "" %>>独立窗口方式</option>
                                                <option value="tab" <%="tab".equals(auth.getCrt_mode()) ? "selected" : "" %>>选项卡方式</option>
                                            </select>
                                         </div>
                                    </td>
                                    <td>&nbsp;
                                        
                                    </td>
                                </tr>
                                <tr id=login_tool_tr>
                                    <td width=150 align="right">登录工具：</td>
                                    <td width=20 align="right"></td>
                                    <td id=login_tool1 width=300>
                                    	<div class="sel">
                                        <select onChange="show_mode(this)" id=login_tool name=login_tool  >
                                            <option value="scrt" <%="scrt".equals(auth.getLogin_tool()) ? "selected" : "" %>>SecureCRT</option>
                                            <option value="putty" <%="putty".equals(auth.getLogin_tool()) ? "selected" : "" %>>PUTTY</option>
                                            <option value="xshell" <%="xshell".equals(auth.getLogin_tool()) ? "selected" : "" %>>XShell</option>
                                        </select>
                                        </div>
                                    </td>
                                    <td>&nbsp;
                                        
                                    </td>
                                </tr>
                                <%} else if(ResCase.TYPE_VM_WIN.equals(auth.getPtype())){ %>
                                
                                 <tr>
                                    <td width=150 align="right">登录方式：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                      <div class="sel">
                                        <select id=sso_type name=sso_type style="font-size:12px; width:280px;" onChange="chg_sso_type()" >
                                            <!-- 根据访问方式  显示option -->
                                            <%=tag.options_v(bean.getSso_type_map(), bean.getAuth().getSso_type()) %>
                                        </select>
                                        </div>
                                    </td>
                                    <td>&nbsp;
                                        
                                    </td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">协议：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <!-- 图形 协议 -->
                                        <% 
                                        String df_protocol = auth.getProtocol();
                                        String df_port = null;
                                        ischk = false;
                                        
                                       	if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_RDP)) { 
											String chk_tag = "";
											if (df_protocol == null || df_protocol.length() == 0 || ResCase.PROTO_RDP.equals(df_protocol)) {
												ischk = true;
												chk_tag = "checked";
												df_port = auth.getRdp_port();
											}
											
											if (df_port == null || df_port.length() == 0) {
												String rdp_port = auth.getRdp_port();
												df_port = (rdp_port == null || rdp_port.length() == 0) ? ResCase.DF_PORT_RDP : rdp_port;
											}
										%>
					    					<div id="rdp" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:10px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_RDP%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_RDP %>');view_param(this.value);"/><span>RDP</span></div>
					    				<% } %>
					    				
					    				<% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_VNC)) {
					    					String chk_tag = ischk ? "" : "checked";
					    					if (df_protocol != null && df_protocol.length() > 0 && ResCase.PROTO_VNC.equals(df_protocol)) {
					    						ischk = true;
					    						chk_tag = "checked";
					    						df_port = auth.getVnc_port();
					    					}
					    					
					    					if (df_port == null || df_port.length() == 0) {
					    						String vnc_port = auth.getVnc_port();
				    							df_port = (vnc_port == null || vnc_port.length() == 0) ? ResCase.DF_PORT_VNC : vnc_port;
				    						}							    					
					    				%>
					    					<div id="vnc" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:10px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_VNC%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_VNC %>');view_param(this.value);"/><span>VNC</span></div>
					    				<% } %>
					    				
					    				<!-- xftp -->
                                        <% 	if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_FTP)) {
											String chk_tag = "";
											if (df_protocol == null || df_protocol.length() == 0 || ResCase.PROTO_FTP.equals(df_protocol)) {
												ischk = true;
												chk_tag = "checked";
												df_port = auth.getFtp_port();
											}
											
											if (df_port == null || df_port.length() == 0) {
												String ftp_port = auth.getFtp_port();
												df_port = (ftp_port == null || ftp_port.length() == 0) ? ResCase.DF_PORT_FTP : ftp_port;
											}
										%>
					    					<div id="ftp" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:10px;" name="protocol" <%=chk_tag %> value="<%=ResCase.PROTO_FTP%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_FTP%>');view_param(this.value);"/><span>FTP</span></div>
					    				<% } %>
					    				
					    				<% if(AuthCase.verify(auth.getAccess_mode(), ResCase.INT_MARK_SMB)) {
					    					String chk_tag = ischk ? "" : "checked";
				    						if (df_protocol != null && df_protocol.length() > 0 && ResCase.PROTO_SMB.equals(df_protocol)) {
				    							ischk = true;
				    							chk_tag = "checked";
					    						df_port = auth.getSmb_port();
					    					}
				    						
					    					if (df_port == null || df_port.length() == 0) {
					    						String smb_port = auth.getSmb_port();
					    						
					    						df_port = (smb_port == null || smb_port.length() == 0) ? ResCase.DF_PORT_SMB : smb_port;
			    							}							    					
					    				%>
					    					<div id="smb" style="float: left; padding-left: 10px;"><input type="radio" style="height:15px; padding-right:10px;" <%=chk_tag %> name="protocol" value="<%=ResCase.PROTO_SMB%>" onclick="chg_port('<%=auth.getRes_rdn() %>', '<%=ResCase.PROTO_SMB%>');view_param(this.value);"/><span>共享目录</span></div>
					    				<% } %>
                                    </td>
                                    <td>&nbsp;
                                        
                                    </td>
                                </tr>
                                <tr>
                                    <td width=150 align="right">端口：</td>
                                    <td width=20 align="right"></td>
                                    <td width=300>
                                        <input id="port" name="port" value="<%=df_port%>" type=text  style="width:280px;"  >
                                    </td>
                                    <td>&nbsp;</td>
                                </tr>
								<tr id=disp_tr>
									<td width=150 align="right">显示分辨率：</td>
									<td width=20 align="right"></td>
									<td id=dpi width=300> 
                                    	 <div class="sel">
                                            <select id=disp name=disp style=" width:280px;">
                                                <option value="full" <%="full".equals(auth.getDisp()) ? "selected" : "" %>>全屏</option>
                                                <option value="800x600" <%="800x600".equals(auth.getDisp()) ? "selected" : "" %>>800 x 600</option>
                                                <option value="1024x768" <%="1024x768".equals(auth.getDisp()) ? "selected" : "" %>>1024 x 768</option>
                                                <option value="1280x720" <%="1280x720".equals(auth.getDisp()) ? "selected" : "" %>>1280 x 720</option>
                                                <option value="1280x768" <%="1280x768".equals(auth.getDisp()) ? "selected" : "" %>>1280 x 768</option>
                                                <option value="1280x800" <%="1280x800".equals(auth.getDisp()) ? "selected" : "" %>>1280 x 800</option>
                                                <option value="1366x768" <%="1366x768".equals(auth.getDisp()) ? "selected" : "" %>>1366 x 768</option>
                                                <option value="1440x900" <%="1440x900".equals(auth.getDisp()) ? "selected" : "" %>>1440 x 900</option>
                                                <option value="1600x900" <%="1600x900".equals(auth.getDisp()) ? "selected" : "" %>>1600 x 900</option>
                                            </select>
                                        </div>
									</td>
									<td>&nbsp;
										
									</td>
								</tr>
								<tr id=driver_tr>
									<td width=150 align="right">共享本地驱动器：</td>
									<td width=20 align="right"></td>
									<td width=300  id="local_drive">
										<div style="overflow: auto;height:100px; width:280px; border:1px solid #000;margin-top:10px;"> 
											<table style="border-collapse: collapse;">
												<tr>
													<td id="driver_td" <%=no_driver ? "disabled" : "" %>>
														<input type="checkbox" value="c" name="driver" style="height:15px; "><span>本地磁盘(C:)</span><br/>
														<input type="checkbox" value="d" name="driver" style="height:15px; "><span>本地磁盘(D:)</span><br/>
														<input type="checkbox" value="e" name="driver" style="height:15px; "><span>本地磁盘(E:)</span><br/>
														<input type="checkbox" value="f" name="driver" style="height:15px; "><span>本地磁盘(F:)</span><br/>
														<input type="checkbox" value="g" name="driver" style="height:15px; "><span>本地磁盘(G:)</span><br/>
														<input type="checkbox" value="h" name="driver" style="height:15px; "><span>本地磁盘(H:)</span><br/>
														<input type="checkbox" value="i" name="driver" style="height:15px; "><span>本地磁盘(I:)</span><br/>
														<input type="checkbox" value="j" name="driver" style="height:15px; "><span>本地磁盘(J:)</span><br/>
														<input type="checkbox" value="k" name="driver" style="height:15px; "><span>本地磁盘(K:)</span>
													</td>
												</tr>
											</table>
										</div>
									</td>
									<td>&nbsp;
										
									</td>
								</tr>
								<tr id=clip_tr>
									<td width=150 align="right">共享剪切板：</td>
									<td width=20 align="right"></td>
									<td width=300>
										<input id="clipboard" name="clipboard" value="1" type="checkbox" <%="1".equals(auth.getClipboard()) ? "checked" : "" %> <%=no_clip ? "disabled" : "" %> style="height:15px; padding-right:10px;"><span>剪切板</span>
									</td>
									<td>&nbsp;
										
									</td>
								</tr>
								<tr id=console_tr>
									<td width=150 align="right">高级：</td>
									<td width=20 align="right"></td>
									<td width=300>
										<input id="is_console" name="is_console" value="1" type="checkbox" <%="1".equals(auth.getIs_console()) ? "checked" : "" %> <%=no_console ? "disabled" : "" %> style="height:15px; padding-right:10px;"><span>使用控制台方式登录</span>
									</td>
									<td>&nbsp;
										
									</td>
								</tr>
                                
                                
                                <%} %>
                                <tr>
                                    <td width=150 align="right" style="padding-bottom:20px;">备注：</td>
                                    <td width=20 align="right" style="padding-bottom:20px;"></td>
                                    <td width=300 style="padding-bottom:20px;">
                                        <textarea rows="4" cols="" id="remark" name="remark" style="width:280px;"><%=auth.getRemark() %></textarea>
                                    </td>
                                    <td>&nbsp;
                                    </td>
                                </tr>
                        </table>
                        <div class="btn_foot" style="margin-left:50px;">
                                <% if (workId != null && workId.length() > 0 ||
                               		!ResCase.RES_SSO_LOGIN_APPROVAL.equals(auth.getApproval())) { %>
                                <button type="button" id="connect" onClick="sso_connect()" style="background: #25b332;border:none;margin-right: 15px;">连 接</button>
                                <% } else { %>
                               	<button type="button" id="application" onClick="sso_application()" style="background: #25b332;border:none;margin-right: 15px;">登录需申请</button>
                               	<% } %>
                                <!-- <button type="button" id="close" onClick="go_back()" style="background: #25b332;border:none;margin-right: 15px;">返 回</button> -->
                                <button type="button" id="save" onClick="save_config()" style="background: #25b332;border:none;margin-right: 15px;">保存为默认</button>
                                <%if(is_save.equals("1") && ResCase.RES_SSO_LOGIN_NOT_APPROVAL.equals(auth.getApproval())) {%>
                                <button type="button" id="favorites" onClick="add_fav()" style="background: #25b332;border:none;margin-right: 15px;">添加到收藏夹</button>
                                <%} %> 
                                <button type="button" id="audit" onClick="link_audit()" style="background: #25b332;border:none;margin-right: 15px;">查看个人审计</button>
                        </div>
					</form>
                 </div>
            </div>
         </div>
	</div>	
</body>
<script type="text/javascript">	
	function $(id){
		return document.getElementById(id);
	}

	var DF_PORT_SSH = "<%=(auth.getSsh_port() == null || auth.getSsh_port().length() == 0) ? ResCase.DF_PORT_SSH : auth.getSsh_port()%>";
	var DF_PORT_TELNET = "<%=(auth.getTelnet_port() == null || auth.getTelnet_port().length() == 0) ? ResCase.DF_PORT_TELNET : auth.getTelnet_port()%>";
	var DF_PORT_FTP = "<%=(auth.getFtp_port() == null || auth.getFtp_port().length() == 0) ? ResCase.DF_PORT_FTP : auth.getFtp_port()%>";
	var DF_PORT_SFTP = "<%=(auth.getSftp_port() == null || auth.getSftp_port().length() == 0) ? ResCase.DF_PORT_SFTP : auth.getSftp_port()%>";
	var DF_PORT_SMB = "<%=(auth.getSmb_port() == null || auth.getSmb_port().length() == 0) ? ResCase.DF_PORT_SMB : auth.getSmb_port()%>";
	var DF_PORT_RDP = "<%=(auth.getRdp_port() == null || auth.getRdp_port().length() == 0) ? ResCase.DF_PORT_RDP : auth.getRdp_port()%>";
	var DF_PORT_VNC = "<%=(auth.getVnc_port() == null || auth.getVnc_port().length() == 0) ? ResCase.DF_PORT_VNC : auth.getVnc_port()%>";
	var DF_PORT_XWIN = "<%=(auth.getXwin_port() == null || auth.getXwin_port().length() == 0) ? ResCase.DF_PORT_XWIN : auth.getXwin_port()%>";
	var PROL_RDP = "<%=ResCase.PROTO_RDP%>";
	var PROL_VNC = "<%=ResCase.PROTO_VNC%>";
	var PROL_XWIN = "<%=ResCase.PROTO_XWIN%>";
	var PROL_SSH1 = "<%=ResCase.PROTO_SSH1%>";
	var PROL_SSH2 = "<%=ResCase.PROTO_SSH2%>";
	var PROL_TELNET = "<%=ResCase.PROTO_TELNET%>";
	var PROL_FTP = "<%=ResCase.PROTO_FTP%>";
	var PROL_SFTP = "<%=ResCase.PROTO_SFTP%>";
	var PROL_SMB = "<%=ResCase.PROTO_SMB%>";
	var ptype = "<%=auth.getPtype() %>";
	var pip = "<%=auth.getPip() %>";
	var res = "<%=auth.getPname() %>";
	var auth_rdn = "<%=bean.getAuth_rdn()%>";
	var acc_name = "<%=auth.getAcc_name()%>";
	var res_rdn = "<%=auth.getRes_rdn()%>";
	var host_rdn = "<%=auth.getResroleRdn() %>";
	var resrole_name = "<%=name %>";
	var df_protocol = "<%=auth.getProtocol()%>";
	var is$User = <%=auth.is$User()%>;
	
	var no_clip = <%=no_clip %>;
	var no_driver = <%=no_driver %>;
	var no_console = <%=no_console %>;
	
	var workId = "<%=workId%>";
	
	var res_menu = "<%=bean.getRes().getMenu() %>";
	var SMODE_ISSUER = "<%=ResCase.SMODE_ISSUER%>";
	var start_mode = "<%=bean.getRes().getStart_mode()%>";
	
	function reset_dfport(protocol, port) {
		if (PROL_SSH1 ==  protocol || PROL_SSH2 == protocol) {
			DF_PORT_SSH = port;
		} else if (PROL_TELNET == protocol) {
			DF_PORT_TELNET = port;
		} else if (PROL_RDP ==  protocol) {
			DF_PORT_RDP = port;
		} else if (PROL_VNC == protocol) {
			DF_PORT_VNC = port;
		} else if (PROL_XWIN == protocol) {
			DF_PORT_XWIN = port;
		} else if (PROL_FTP ==  protocol) {
			DF_PORT_FTP = port;
		} else if (PROL_SFTP == protocol) {
			DF_PORT_SFTP = port;
		} else if (PROL_SMB == protocol) {
			DF_PORT_SMB = port;
		}
	}
	
	function check_simp_eor(rs) {
		if(rs.rv.indexOf("error:") != -1){
			alert(rs.msg);
			return false;
		}
		
		return true;
	}
	
	function sso_connect(){
		if (!verify_port()) {
			return;
		}
		
		/*start yw 2017年3月6日        处理密码中包含的特殊字符*/
		var passwd = document.getElementById("acc_pwd");
		if(passwd != null && passwd != undefined){
			
			var base64 = new Base64(); 
			base64.input_pwd_code();
		}
		/*end yw 2017年3月6日     处理密码中包含的特殊字符  */
		
		var window_rv = true;
		
		try {
			var protocol = get_protocol();
			
			if (PROL_XWIN == protocol || PROL_VNC == protocol) {
				return vnc_xwin(protocol);
			} else if (PROL_RDP == protocol){
				return rdp(protocol);
			} else if (PROL_SMB == protocol || PROL_FTP == protocol || PROL_SFTP == protocol) {
				return xftp(protocol);
			} else if (PROL_SSH1 ==  protocol || PROL_SSH2 == protocol || PROL_TELNET == protocol) {
				var login_tool = $("login_tool").value;
				if("scrt" == login_tool){
					return scrt(protocol);
				}else if("putty" == login_tool){
					return putty(protocol);
				} else if ("xshell" == login_tool) {
					return xshell(protocol);
				}
			} else {
				alert("登录错误");
				window_rv = false;
				return;
			}
		} finally {
			/*start yw 2017年3月6日        解密回显*/
			if(passwd != null && passwd != undefined){
				var base64 = new Base64(); 
				passwd.value = base64.decode(passwd.value);
			}
			/*end yw 2017年3月6日      解密回显 */
			
			window.returnValue = window_rv;  //登录审批单点登录时 使用该值确定是否成功登录
		}
	}
	
	function get_protocol() {
		var rv = "";
		var protocol = document.getElementsByName("protocol");
		
		for (var i = 0; i < protocol.length; i++) {
			if(protocol[i].checked) {
				rv = protocol[i].value;
				break;
			}
		}
		
		return rv;
	}
	
	function xshell(protocol) {
		var fort_ip = get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var port = $("port").value;

		var acc_name = $("acc_name").value;
		var remark = $("remark").value;
		var login_tool = $("login_tool").value;
		var args = "method=login&auth_rdn="+auth_rdn + "&acc_name=" + acc_name;
		args+="&type=cmd&protocol="+protocol+"&port="+port+"&fort_ip="+fort_ip+"&login_tool="+login_tool+"&remark="+remark;
		if (is$User) {
			var pwd = $("acc_pwd").value;
			args+="&acc_pwd=" + pwd;
		}
		
		var xml_doc = new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		
		var rs = xml_doc.result();
		
		if(!check_simp_eor(rs))  {
			return ;
	   	}
		
		/*
		//sso.run( "xshell_agent.exe", "00123456 root root 192.168.23.104 22 测试标签名" );
		var sso_url =  rs.rv + " user "+ rs.rv +" " + fort_ip + " 22" + " " + acc_name + "@" + pip;
		try{
			sso.run("xshell_agent.exe", sso_url);
		} catch(err) {
			alert("客户端控件未安装或被禁用!");
		}
		*/
		if (acc_name == null || acc_name.length == 0) {
			acc_name = "";
		}
		
		try{
			sso_0.xshell_1(fort_ip,"22","user",rs.rv, acc_name + "@" + pip, pip, acc_name);
		} catch(err) {
			alert("客户端控件未安装或被禁用!");
		}
	}
	
	function putty(protocol) {
		var fort_ip = get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var port = $("port").value;

		var acc_name = $("acc_name").value;
		var remark = $("remark").value;
		var login_tool = $("login_tool").value;
		var args = "method=login&auth_rdn="+auth_rdn + "&acc_name=" + acc_name;
		args+="&type=cmd&protocol="+protocol+"&port="+port+"&fort_ip="+fort_ip+"&login_tool="+login_tool+"&remark="+remark;
		if (is$User) {
			var pwd = $("acc_pwd").value;
			args+="&acc_pwd=" + pwd;
		}
		
		var xml_doc = new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		
		var rs = xml_doc.result();
		
		if(!check_simp_eor(rs))  {
			return ;
	   	}
		
		//sso.run("sso_putty.exe","-ssh -P 22 -pw ROOTROOT root@192.168.23.104");
		var sso_url = "-ssh -P 22 -pw "+ rs.rv +" user@" + fort_ip;
		try{
			sso.run("sso_putty.exe", sso_url);
		} catch(err) {
			alert("客户端控件未安装或被禁用!");
		}
	}
	
	function rdp(protocol) {
		var fort_ip = get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var port = $("port").value;
		
		var clipboard = $("clipboard");
		var dis = $("disp").value;
		
		var clip = "clip_off";
		var clip_num = "0";
		if (!no_clip && clipboard.checked) {
			clip = "clip_on";
			clip_num = "1";
		}
		
		var driver_disk = document.getElementsByName("driver");
		
		var	driver = "";
		if (!no_driver) {
			for (var i = 0; i < driver_disk.length; i++) {
				if (driver_disk[i].checked) {
					driver += driver_disk[i].value + ",";
				}
			}
		}
		
		if (driver.length > 0) {
			driver = driver.substring(0, driver.length - 1);
		} else {
			driver= "driver_off";
		}
		
		var acc_name = $("acc_name").value;
		var remark = $("remark").value;
		
		var is_console = $("is_console");
		var console_switch = !no_console && is_console != null && is_console.checked;
		var console = "console_off";
		if (console_switch) {
			console = "console_on";
		}
		var args = "method=login&auth_rdn="+auth_rdn+"&acc_name="+acc_name;
		args+="&type=graphic&protocol="+protocol+"&fort_ip="+fort_ip+"&port="+port+"&disp="+dis+"&drivers="+driver+"&clip="+clip_num+"&remark="+remark;
		if (is$User) {
			args+="&acc_pwd=" + $("acc_pwd").value;
		}
		if (console_switch) {
			args += "&is_console=1";
		}
		
		var xml_doc = new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		var rs = xml_doc.result();
			
		var tool_ext = dis + " " + clip + " " + driver + " " + console;
		if(!check_simp_eor(rs, "mstsc_ex", tool_ext)) {
			return ;
		}
		
		//mstsc_ex.exe sso 192.168.37.20 3390 1392007217408 title 1280x800 clip_on e console_off
		var title = pip + "("+acc_name+")";
		var sso_url = "sso " + fort_ip + " 3390 " + rs.rv + " " + title + " " + dis + " " + clip + " " + driver + " " + console;
		
		try{
			sso.run("mstsc_ex.exe", sso_url);
		}catch(err){
		       alert("客户端控件未安装或被禁用!");
		}
	}
	
	function scrt(protocol) {
		var fort_ip = get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var crt_mode = $("crt_mode").value;	//tab|独立窗口
		
		var port = $("port").value;
		
		var acc_name = $("acc_name").value;
		
		var remark = $("remark").value;

		var login_tool = $("login_tool").value;
		
		var args = "method=login&auth_rdn="+auth_rdn + "&acc_name=" + acc_name;
		
		args+="&type=cmd&protocol="+protocol+"&port="+port+"&fort_ip="+fort_ip + "&crt_mode=" + crt_mode + "&login_tool="+login_tool+"&remark=" + remark;
		
		if (is$User) {
			var pwd = $("acc_pwd").value;
			args+="&acc_pwd=" + pwd;
		}
		
		var xml_doc = new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		
		var rs = xml_doc.result();
		
		if(!check_simp_eor(rs))  {
			return ;
	   	}
		
		var title = res + "(" + pip + ")->" + acc_name;
		
		var sso_url = rs.rv + " ssh2 " + fort_ip + " 22 user " + rs.rv + " " + title;
		
		if (crt_mode != null && crt_mode == "tab"){ 
			try{
				sso_url = sso_url + " -tab";
				sso.run("scrt_agent.exe", sso_url);
			} catch(err) {
				alert("您所使用的SecureCRT版本不支持以选项卡方式打开");
			}
		} else {
			try {
				sso.run("scrt_agent.exe", sso_url);
			} catch(err) {
		       		alert("客户端控件未安装或被禁用!");
		   	}
		}
	}
	
	/**
	* vnc和xwin使用相同客户端SSO
	*/
	function vnc_xwin(protocol) {
		
		var fort_ip = get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var port = $("port").value;
		
		var acc_name = $("acc_name").value;
		
		var remark = $("remark").value;
		
		var args = "method=login&auth_rdn="+auth_rdn+"&acc_name="+acc_name;
		
		args+="&type=graphic&protocol="+protocol+"&port="+port+"&fort_ip="+fort_ip + "&remark=" + remark;
		if (is$User) {
			var pwd = $("acc_pwd").value;
			args+="&acc_pwd=" + pwd;
		}
		
		var xml_doc = new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		var rs = xml_doc.result();
		
		if(!check_simp_eor(rs))  {
			return ;
	   	}

		//mstsc_vnc.exe 192.168.23.95:3390 -run=sso -sid=123456 -title=123 -wsize=800x600
		var title = pip + "("+acc_name+")";
		var sso_url = fort_ip + ":3390 -run=sso -sid=" + rs.rv + " -title=" + title + " -wsize=full";
		
		try{
	    	sso.run("mstsc_vnc.exe", sso_url);
	   	}catch(err){
	      	alert("客户端控件未安装或被禁用!");
	   	}
	}
	
	function xftp(protocol){

		if (!verify_port()) {
			return;
		}
		
		var fort_ip=get_fort_ip();
		
		if (fort_ip == null) {
			return;
		}
		
		var port = $("port").value;

		var acc_name = $("acc_name").value;
		
		var remark = $("remark").value;
		
		var args = "method=login&auth_rdn="+auth_rdn + "&acc_name=" + acc_name;
		args+="&type=xftp&protocol="+protocol+"&port="+port+"&fort_ip="+fort_ip + "&remark=" + remark;
		if (is$User) {
			var pwd = $("acc_pwd").value;
			args+="&acc_pwd=" + pwd;
		}
		
		var xml_doc=new XMLDoc("/iam/sso.do");			
		xml_doc.post(args);
		var rs=xml_doc.result();
		if(!check_simp_eor(rs))  {
			return ;
	   	}
		
		var s_port=20021;
		var title=res+"("+pip+")->"+acc_name;

		var cmd='javaw -Xms256m -Xmx256m -cp simp.ftp.2.0.jar com.simp.app.ft.ui.MainFrame ' + fort_ip + ' ' +s_port+' ' + rs.rv + ' ' + title + ' ' + protocol;
		
		try{
			sso.run("java_ftp.exe", cmd);
    	} catch(err){
        	alert("客户端控件未安装或被禁用!");
    	}
	}
	
	function chg_sso_type() {
		var sso_type = $("sso_type").value;
		var prol = "";
		var hasChk = "<%=ischk%>";
			
		if ("cmd" == sso_type) {
			prol = "ssh2,ssh1,telnet";
		} else if ("graphic" == sso_type) {
			prol = "rdp,vnc,xwin";
		} else if ("xftp" == sso_type) {
			prol = "ftp,sftp,smb";
		} else {
			return ;
		}

		if (df_protocol == "") {
			var hasChk = false;
		} else {
			var hasChk = prol.indexOf(df_protocol) != -1;
		}
		
		var protocols = document.getElementsByName("protocol");
		for (var i = 0; i < protocols.length; i++) {
			if (prol.indexOf(protocols[i].value) != -1) {
				$(protocols[i].value).style.display = "";
				if (protocols[i].value == df_protocol) {
					protocols[i].checked = "checked";
					chg_port(res_rdn, protocols[i].value);
				}
				
				if (!hasChk) {
					protocols[i].checked = "checked";
					hasChk = true;
					chg_port(res_rdn, protocols[i].value);
				}
			} else {
				$(protocols[i].value).style.display = "none";
			}
		}
	}
	
	function chg_port(res_rdn, prol) {
		var port = document.getElementById("port");
		var post_data = "method=get_port&res_rdn=" + res_rdn + "&protocol="+prol;
		var xml_doc = new XMLDoc("/iam/ssocnfig.do");		
		xml_doc.post(post_data);
		
		var rs = xml_doc.result();
		if (rs.rv.indexOf("error") != -1 || rs.rv == "err") {
			if (prol == PROL_SSH1 || prol == PROL_SSH2) {
				port.value = DF_PORT_SSH;
			} else if (prol == PROL_TELNET) {
				port.value = DF_PORT_TELNET;
			} else if (prol == PROL_FTP) {
				port.value = DF_PORT_FTP;
			} else if (prol == PROL_SFTP) {
				port.value = DF_PORT_SFTP;
			} else if (prol == PROL_SMB) {
				port.value = DF_PORT_SMB;
			} else if (prol == PROL_RDP) {
				port.value = DF_PORT_RDP;
			} else if (prol == PROL_VNC) {
				port.value = DF_PORT_VNC;
			} else if (prol == PROL_XWIN) {
				port.value = DF_PORT_XWIN;
			}
		} else {
			port.value = rs.rv;
		}
	}
	
	function chg_view() {
		var sso_type = $("sso_type").value;
		
		if ("cmd" == sso_type) {
			<% if (ResCase.TYPE_VM_UNIX.equals(auth.getPtype())){ %>
				$("crt_mode_tr").style.display = "";
				$("login_tool_tr").style.display = "";
			<%}%>
		} else if ("graphic" == sso_type || "xftp" == sso_type) {
			<% if (ResCase.TYPE_VM_UNIX.equals(auth.getPtype())){ %>
				$("crt_mode_tr").style.display = "none";
				$("login_tool_tr").style.display = "none";
			<%}%>
		}
	}
	
	chg_sso_type();
	chg_view();
	
	function go_back() {
		if(workId.length > 0){
			location.href = "/iam/portal/order/order_list.jsp";
		}else{
			location.href = "/iam/portal/sso/auth_list.jsp?host_rdn=" + host_rdn + "&name=" + resrole_name;
		}
	}
	
	function sso_application() {
// 		if (!confirm("该授权登录需申请.是否申请?")) {
// 			return;
// 		}
		
		//跳转登录审批申请页面  并代填部分信息
		
		alert("该授权登录需申请.");
	}

	function show_mode(sel){
		var crt_mode = $("crt_mode");
		if("putty" == sel.value){
			crt_mode.disabled=true;
		}else{
			crt_mode.disabled=false;
		}
	}
	
</script>
</html>
