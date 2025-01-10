<%@ page language="java" contentType="text/html; charset=gbk" pageEncoding="gbk"%>
<%@ page import="com.simp.view.tag.*"%>
<%@ page import="com.simp.biz.res.ResCase"%>
<%@ page import="com.simp.action.RTUser"%>
<%@ page import="com.simp.biz.policy.*"%>
<%@ page import="com.simp.util.jsp.JspTool"%>
<%@ page import="java.util.*"%>
<%@page import="com.simp.biz.role.Role"%>
<%@page import="com.simp.util.jsp.Parameter"%>
<%@page import="com.simp.util.txt.*"%>
<html>
<head>
<title>数据库资源编辑</title>
<link rel="stylesheet" type="text/css" href="/pub/css5/pub5.css"/>
<link rel="stylesheet" type="text/css" href="/pub/css5/chrome5.css" media="screen" />
<style type="text/css">
	.tb{border-collapse: collapse;}
	.tb1{border-collapse: collapse; width:100%; height:100%;}
</style>
<script type="text/javascript" src="/pub/js5/public.js"></script>
<script type="text/javascript" src="/pub/js5/stretch.js" defer="defer"></script>
<script type="text/javascript" src="/pub/js5/XMLDoc.js"></script>
<script type="text/javascript" src="/pub/js5/validate.js"></script>
<script type="text/javascript" src="/pub/js5/meta_d/db.js"></script>
<script type="text/javascript" src="/pub/js5/meta_d/app_sel_forder.js"></script>
<jsp:include page="/inc/pop_view.jsp"></jsp:include>
<script type="text/javascript">
	function go(dst){
		var pf = get_parent_frame("main_frame");
		pf.location.href="/iam/meta_d/res/"+dst+".jsp";
	}
</script>
</head>
<%
	Tag tag=new Tag(request);
	String start_mode_v = tag.v("start_mode");
	
	String hid = start_mode_v == null || start_mode_v.length() == 0 || ResCase.SMODE_ISSUER.equals(start_mode_v) ? "display:" : "display:none";
	String rv_msg=tag.v("msg");
	
	String ref_vm = tag.v("ref_vm");
	
	RTUser rtUser=RTUser.get_from_session(request);
	
	String host_dn = rtUser.getTreeCurRdn();
	
	String status = tag.v("status");
	
	Policy pol = new Policy();
	pol.ini_pol_map(null, host_dn);
	
	String rdn = tag.v("rdn","") ;
	boolean is_add = rdn.length()==0 ? true : false ;
	
	String type_readonly = "";
	String status_enable = "";
	
	if (!is_add) {
		type_readonly = "readonly";
		
		if (ResCase.STATUS_OFF.equals(status)) {
			status_enable = "disabled";
		}
	}
	
	//添加时不显示除保存之外操作按钮
	String res_sn = tag.v("sn");
	String res_display = "block";
	if(res_sn == null || res_sn.length() == 0) {
		res_display = "none";
	}

	Map<String, String> status_map = ResCase.status_map();
	
	if(is_add){
		status_map = new LinkedHashMap<String, String>();
		status_map.put(ResCase.STATUS_ON, "上线");
	}
	
	/*
		从HOME页进入传值判断
	*/
	String type = tag.v("type");
	
	String type_value = Parameter.CharStr(request, "type_value");
	
	if(type_value != null && type_value.length()>0) {
		type = type_value;
	}
	
	if(type == null || type.length()==0) {
		type = ResCase.TYPE_ORACLE;
	}
	
	String display = "block" ;
	if(type ==null || !type.endsWith(ResCase.SUFFIX_UNIX)){
		display = "none";
	}
	
	String ref_res_ip_name = tag.v("ref_res_ip_name");
	
	String ref_res_disabled = "disabled";
	
	if (ref_res_ip_name == null || ref_res_ip_name.length() == 0) {
		ref_res_disabled = "";
	}
	
	if(type ==null || !type.endsWith(ResCase.SUFFIX_SYSDB)){
		ref_res_disabled = "disabled";
	}else if(type.endsWith(ResCase.SUFFIX_SYSDB)){
		ref_res_disabled = "";
	}
	
	String start_mode = tag.v("start_mode");
	if (start_mode == null || start_mode.length() == 0) {
		start_mode = ResCase.SMODE_ISSUER;
	}
	
	String domian_rdn = RdnTool.get_domain_rdn(host_dn);
	
	String db_name_lab_display = ResCase.MARK_SERVICE_CHKED.equals(tag.v("markchked")) ? "none" : "";
	String db_service_name_disabled = ResCase.MARK_SERVICE_CHKED.equals(tag.v("markchked")) ? "" : "disabled";
	String db_service_name_lab_display = ResCase.MARK_SERVICE_CHKED.equals(tag.v("markchked")) ? "" : "none";
	String markchk_chked = ResCase.MARK_SERVICE_CHKED.equals(tag.v("markchked")) ? "checked" : "";
	
%>
<%
	//save msg
if(rv_msg.length()>0){
%>
	<pre id="simp_act_rv_msg" style="display:none"><%=rv_msg%></pre>
	<script type="text/javascript">	
		window.setTimeout("alert(simp_act_rv_msg.innerText)",1);
	</script>
<%
	}
%>
<body>
	<form name=fm method=post onsubmit="return false;">
	<table class="tb1" style="border: 1px solid #d3d3d3;">
		<tr height=30>
			<td>
				<div style="width:100%">
					<table class="nav" style="border-bottom:1px solid #d3d3d3;">
						<tr>
							<td>
							<img src="/pub/img5/public/home.png" class="home"/>
							<span class="home-font"><%=rtUser.getTreeCurNodeName() %></span>
							<img src="/pub/img5/public/next.png" class="next"/>
							<span class="next-font" onclick="go('home');">资源</span>
							<img src="/pub/img5/public/next.png" class="next"/>
							<span class="next-font" onclick="go('db/list');">数据库列表</span>
							<img src="/pub/img5/public/next.png"  class="next"/>
							<span class="next-font">编辑</span>
							<img style="margin:10 0 0 15; cursor: hand;" src=/pub/img5/public/btn_back_16.png onclick="go_back_refresh()" title="返回" />
							</td>
						</tr>
					</table>
				</div>
			</td>
		</tr>
		<tr>
			<td valign="top">
				<div style="width: 100%; height:100%; overflow:auto; ">
					<table class="tb1">
						<tr>
							<td width=50>
							</td>
							<td valign="top">
								<table class="tb1">
									<tr height=10>
										<td style="font-size: 0px;">
											
										</td>
									</tr>
									<tr height=70>
										<td>
											<table style="border-collapse: collapse; width:90%;">
												<tr>
													<td align="center" valign="top" <%=status_enable %>>
														<table style="width: 100%;border-collapse: collapse;">
															<tr>
																<td style=" width:80px;"  align="left">
																	<img id="type_img" src="/pub/img5/meta_d/res/<%=tag.v("type", type) %>.png"/>
																</td>
																<td width=180>
																	<table style="width: 100%;border-collapse: collapse;">
																		<tr>
																			<td colspan="3">
																				<span style="font-weight: bold;">类 型：</span>
																				<span id="type_lab"><%=ResCase.res_type_des_map().get(tag.v("type", type)) %></span>
																			</td>
																		</tr>
																		<tr>
																			<td  width=80>
																				<span style="font-weight: bold;">
																					状 态：
																				</span>
																			</td>
																			<td width=250 align="left">
																				<%
																					if(ResCase.STATUS_ON.equals(tag.v("status", ResCase.STATUS_ON))) {
																				%>
																					<div style="background: url('/pub/img5/meta_d/res/ping.png'); height:25px; width:45px;  color:#Fff; border:0px; margin-left:5px;line-height: 25px; text-align: center;">
																						上线
																					</div>
																				<%
																					} else {
																				%>
																					<div style="background: url('/pub/img5/meta_d/res/down.png'); height:25px; width:45px;  color:#Fff; border:0px;margin-left:5px;line-height: 25px; text-align: center;">
																						下线
																					</div>
																				<% } %>
																			</td>
																			<td>
																				&nbsp;
																			</td>
																		</tr>
																	</table>
																</td>
																<td>&nbsp;</td>
															</tr>
														</table>
													</td>
												</tr>
											</table>
										</td>
									</tr>
									<tr height=2>
										<td style="border-bottom: 1px dashed #d3d3d3; font-size: 0px;">
											&nbsp;
										</td>
									</tr>
									<tr height=10>
										<td>
											
										</td>
									</tr>
									<tr height=70>
										<td style="background: #fdfdfd; border:1px solid #eee;">
											<table class="tb1">
												<tr height=15>
													<td style="font-size: 0px;">
													</td>
												</tr>
												<tr>
													<td valign="top">
														<div style="width:530px;">
															<table class="tb">
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		类型：
																	</td>
																	<td width=10 height=30></td>
																	<td>  
																		<select tabindex="1"  class="edit_input_text" id=type name="type" onchange="chg_type(this)" >
																			<%=tag.options_v(ResCase.get_db_type_map(), type) %>
																		</select>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		名称：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input type=hidden name="rdn" readonly value="<%=tag.v("rdn")%>">
																		<input type=hidden name="menu" value="<%=tag.v("menu",ResCase.MENU_TYPE_DB)%>" class="text">
													    					<input tabindex="3" type=text name="name" value="<%=tag.v("name")%>" class="edit_input_text">&nbsp;<label style="color: red;">*</label>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right">
																		归属组：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="6" readonly type="text" name="groupName" id="groupName" value="<%=tag.name_path("groupRdn",rtUser.getTreeCurRdn()) %>" style="float:left;width: 300px;"/>
																		<input type=hidden name="groupRdn" value="<%=tag.v("groupRdn",rtUser.getTreeCurRdn()) %>" class="text"/>
																		<img tabindex="7" style="cursor:hand; margin-top:3px; margin-left:3px;" name="group_select" id="group_select" src="/pub/img5/public/find_16.png" onclick="dep_sel_dlg()" title="选择"/>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		库名称：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="9" type=text name="db_name" value="<%=tag.v("db_name")%>" class="text edit_input_text"/>&nbsp;<label style="display:<%=db_name_lab_display %>" id="db_name_lab" style="color: red;">*</label>
																	</td>
																</tr>
<%-- 																<% if(ResCase.TYPE_ORACLE.equals(type)) {%> --%>
																<tr id="service_tr">
																	<td width=40 height=30 align="right">&nbsp;</td>
																	<td width=85 align="right" align="right">
																		服务名：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="11" type=text name="db_service_name" <%=db_service_name_disabled %> value="<%=tag.v("db_service_name")%>" class="text edit_input_text"/>&nbsp;<label style="display:<%=db_service_name_lab_display %>"  id="db_service_name_lab" style="color: red;">*</label>
																	</td>
																</tr>
																<tr id="mark_tr">
																	<td width=40 height=30 align="right">&nbsp;</td>
																	<td width=85 align="right" align="right">
																		标识选择：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="12" type="checkbox" name="markchk" id="markchk" <%=markchk_chked %> value="1" onclick="text_change(this)"/>
																		<input type="hidden" name="markchked" id="markchked" value='<%=tag.v("markchked") %>' />
																		使用服务名作为服务标识
																	</td>
																</tr>
<%-- 																<% } %> --%>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		端口：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="13" type=text  name="port" value="<%=tag.v("port")%>" class="text edit_input_text" >&nbsp;<label style="color: red;">*</label>
																		<img tabindex="14" src=/pub/img5/meta_d/res/edit/port_test_16.png onclick="test_port()" title="测试" style="cursor: hand;margin-top: 2px;"/>
																		<span id="port_status" style="width:60px; height:20px; line-height: 16px;"></span>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td align="right">
																		启动方式：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<table>
																			<tr>
																				<td style="width:20px;padding-bottom:4px;"><input tabindex="15"  onclick="chg_start_mode(true)" type="radio" name="start_mode" value="<%=ResCase.SMODE_ISSUER %>" <%=tag.check_v(start_mode, ResCase.SMODE_ISSUER) %>></td>
																				<td>应用发布</td>
																				<td style="width:20px;padding-bottom:4px;"><input tabindex="16"  onclick="chg_start_mode(false)" type="radio" id="direct_mode" name="start_mode" value="<%=ResCase.SMODE_DIRECT %>" <%=tag.check_v(start_mode, ResCase.SMODE_DIRECT) %>></td>
																				<td>客户端直连</td>
																			</tr>
																		</table>
																	</td>
																</tr>
																<tr id="tr_issuers" style="<%=hid%>">
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		应用发布：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<table style="width: 330px;">
																			<tr>
																				<td style="width: 300px;">
																					<select tabindex="17" name="app_svr_acc_rdn_set" id="app_svr_acc_rdn_set" style="width: 100%;" multiple="multiple">
																						<%=tag.optionsMap("app_svr_acc_map") %>
																					</select>
																				</td>
																				<td>&nbsp;<label style="color: red;">*</label></td>
																				<td>&nbsp;</td>
																				<td>
																					<img tabindex="18" src=/pub/img5/public/btn_add_16.png title="添加" onclick="app_sel_dlg()" style="cursor: hand"/>
																					<img tabindex="19" src=/pub/img5/public/btn_del_16.png title="删除" onclick="del_options(fm.app_svr_acc_rdn_set);" style="cursor: hand"/>
																					<img tabindex="20" style="cursor:hand;" src="/pub/img5/secpol/up_16.png" title="上移" onmousedown="setTimeStart('up');" onmouseup="clearTimeout(x);" 
																					onclick="listObj=document.getElementById('app_svr_acc_rdn_set');upListItem();clearTimeout(x);"/> 
									               											<img tabindex="21" style="cursor:hand;" src="/pub/img5/secpol/down_16.png" title="下移" onmousedown="setTimeStart('down');"
																					onmouseup="clearTimeout(x);" onclick="listObj=document.getElementById('app_svr_acc_rdn_set');downListItem();clearTimeout(x);"/> 
																				</td>
																			</tr>
																		</table>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td align="right">
																		登录方式：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<select tabindex="22"  class="edit_input_text" name="sso_mode">
																			<%=tag.options(ResCase.get_sso_mode_map(), "sso_mode")%>
																		</select>
																	</td>
																</tr>
															</table>
														</div>
													</td>
													<td valign="top">
														<div style="width:580px;">
															<table class="tb">
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		状态：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<select tabindex="2" name=status onchange="status_prompt('<%=ResCase.STATUS_OFF%>',this,'<%=ResCase.STATUS_ON %>')"  class="edit_input_text" >
																			<%=tag.options(status_map, "status")%>
																		</select>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		IP：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="4" type=text name="ip" value="<%=tag.v("ip")%>"  class="edit_input_text">&nbsp;<label style="color: red;">*</label>
																		<button type="button" tabindex="5" onclick="test_ip()" title="测试连通性" style="background: url('/pub/img5/meta_d/res/ping.png'); height:25px; width:45px;  color:#Fff; border:0px;cursor: hand; margin-left:5px;">
																			ping
																		</button>
																		<span id="ip_status" style="width:60px; height:20px; line-height: 16px;"></span>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		网络服务名：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<input tabindex="8" type=text  name="db_svr_name" value="<%=tag.v("db_svr_name")%>" class="text edit_input_text"/>
																	</td>
																</tr>
																<tr>
																	<td width=40 height=30></td>
																	<td width=85 align="right" align="right">
																		口令策略：
																	</td>
																	<td width=10 height=30></td>
																	<td>
																		<select tabindex="10" name=pwdPolRdn   class="edit_input_text" >
																			<option value=""/>
																			<%=tag.options(pol.get_pwd_map(), "pwdPolRdn") %>
																		</select>
																	</td>
																</tr>
															</table>
														</div>
													</td>
												</tr>
											</table>
										</td>
									</tr>	
									<tr height=10>
										<td></td>
									</tr>
									<tr height=115>
										<td style="background: #fdfdfd; border:1px solid #eee;">
											<table class="tb" style="width: 100%;">
												<tr>
													<td  valign="top">
													<div style="width:530px;">
													<table class="tb">
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																是否需审批
															</td>
															<td width=10 height=30></td>
															<td>
																<input type="checkbox" id=approval_chkbox <%=tag.check("approval", ResCase.RES_SSO_LOGIN_APPROVAL) %> onclick="add_approval()"/>
																<span style="color:#5B5B5B; font-size: 12px;">勾选状态表示该资源账号单点登录需审批。</span>
																<input type="hidden" name="approval" id="approval" value="<%=tag.v("approval", ResCase.RES_SSO_LOGIN_NOT_APPROVAL) %>">
															</td>
														</tr>
														<tr id="approver_first">
															<td width=40 height=30></td>
															<td width=85 align="right">
																审批人1：
															</td>
															<td width=10 height=30></td>
															<td>
																<input type="text" class="edit_input_text" readonly="readonly" name="approver1" id="approver1" value="<%=tag.v("approver1")%>" />
																<img style="cursor:hand; margin-top:3px; margin-left:3px;" name="approval_select" src="/pub/img5/public/find_16.png" onclick="approval_sel('first')" title="选择"/>
															</td>
														</tr>
														
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																归属主机：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="23" readonly type=text name="ref_res_ip" id="ref_res_ip" <%=ref_res_disabled %> value="<%=ref_res_ip_name %>" class="edit_input_text" style="width: 300px;" >
																<input type=hidden name="ref_res" value="<%=tag.v("ref_res")%>" >
																<input type=hidden name="res_rdn" value="<%=tag.v("res_rdn")%>" >
																<img tabindex="24" src=/pub/img5/meta_d/res/edit/sel_16.png onclick="ref_host_sel_dlg()" title="选择" style="cursor: hand"/>
																<img tabindex="25" src=/pub/img5/public/btn_del_16.png onclick="ref_host_clean()" title="清空归属主机" style="cursor:hand;"/>
															</td>
														</tr>
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																数据库版本：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="27" type="text"  class="edit_input_text" name="sys_ver"value="<%=tag.v("sys_ver")%>"/>
															</td>
														</tr>
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																所属部门：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="29" type="text"  class="edit_input_text" name="attach_to" value="<%=tag.v("attach_to")%>"/>
															</td>
														</tr>
													</table>
													</div>
												</td>
												<td valign="top">
													<div style="width:580px;">
														<table class="tb">
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
															</td>
															<td width=10 height=30></td>
															<td>
															</td>
															<td width=110></td>
														</tr>
														<tr id="approver_second">
															<td width=40 height=30></td>
															<td width=85 align="right">
																审批人2：
															</td>
															<td width=10 height=30></td>
															<td>
																<input type="text" class="edit_input_text" readonly="readonly" name="approver2" id="approver2" value="<%=tag.v("approver2")%>" />
																<img style="cursor:hand; margin-top:3px; margin-left:3px;" name="app_select" src="/pub/img5/public/find_16.png" onclick="approval_sel('second')" title="选择"/>
															</td>
															<td width=110></td>
														</tr>
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																物理位置：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="26" type="text"  class="edit_input_text" name="position"value="<%=tag.v("position")%>" />
															</td>
															<td width=110></td>
														</tr>
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																厂商：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="28" type="text"  class="edit_input_text" name="vender"value="<%=tag.v("vender")%>" />
															</td>
														</tr>
														<tr>
															<td width=40 height=30></td>
															<td width=85 align="right">
																规格：
															</td>
															<td width=10 height=30></td>
															<td>
																<input tabindex="30" type="text" class="edit_input_text" name="dimension" value="<%=tag.v("dimension")%>"/>
															</td>
														</tr>
													</table>
													</div>
												</td>
												</tr>
											</table>
										</td>
									</tr>
									<tr height=10>
										<td></td>
									</tr>
									<tr height=80>
										<td style="background: #fdfdfd; border:1px solid #eee;">
											<table class="tb" style="width:100%;">
												<tr>
													<td width=40 height=30></td>
													<td width=85 align="right" align="right">
														描述：
													</td>
													<td width=10 height=30></td>
													<td>
														<textarea tabindex="31" name="desc" style="width:300px; height:60px;"><%=tag.v("desc")%></textarea>
													</td>
												</tr>
											</table>
										</td>
									</tr>
									<tr height=50>
										<td valign="bottom">
											<table class="tb">
												<tr>
													<% if(is_add || ResCase.STATUS_ON.equals(status)) { %>
														<% if ((rtUser.rc_inner(Role.TYPE_KEY_DB, Role.RES_ADD) &&  is_add) ||  rtUser.rc_inner(Role.TYPE_KEY_DB, Role.RES_MOD)) {%>
														<td style="width:105px;">
															<button type="button" tabindex="32" onclick="save()" style="width:100px; background:#fff; border:0px;">
																<div style="background: url('/pub/img5/public/btn/btn_green_100.png'); font-size:12px;  color: #fff; width:100px; height:30px; text-align: center;cursor: hand; line-height: 30px; ">
																	保存
																</div>
															</button>
														</td>
														<% } %>
													<% } %>
													<td style="width: 105px; " >
														<button type="button" tabindex="33" onclick="go_back_refresh()" style="width:100px; background:#fff; border:0px;">
															<div style="background: url('/pub/img5/public/btn/btn_gray_100.png'); font-size:12px;  color: #fff; width:100px; height:30px; text-align: center;cursor: hand; line-height: 30px; ">
																关闭
															</div>
														</button>
													</td>
													<td style="width: 105px;">
														<div style="display:<%=res_display %>">
														<button type="button" tabindex="34" onclick="go_acclist()" style="width:100px; background:#fff; border:0px;">
															<div style="background: url('/pub/img5/public/btn/btn_orange_100.png'); font-size:12px;  color: #fff; width:100px; height:30px; text-align: center;cursor: hand; line-height: 30px; ">
																账号
															</div>
														</button>
														</div>
													</td>
													<td>&nbsp;</td>
												</tr>
											</table>
										</td>
									</tr>
									<tr>
										<td>
											
										</td>
									</tr>
								</table>
							</td>
							<td width=50>
							</td>
						</tr>
					</table>
				</div>
			</td>
		</tr>
	</table>							
</form>
<script type="text/javascript">
	var STATUS_OFF = "<%=ResCase.STATUS_OFF%>";
	var STATUS_ON = "<%=ResCase.STATUS_ON%>";
	var domian_rdn= "<%=domian_rdn %>";
	var TYPE_ORACLE = "<%=ResCase.TYPE_ORACLE%>";
	var SUFFIX_SYSDB= "<%=ResCase.SUFFIX_SYSDB%>";
	var MARK_SERVICE_CHKED = "<%=ResCase.MARK_SERVICE_CHKED%>";
	var MARK_SERVICE_UNCHKED = "<%=ResCase.MARK_SERVICE_UNCHKED%>";
	var TYPE_MYSQL = "<%=ResCase.TYPE_MYSQL%>";
	var TYPE_DB2 = "<%=ResCase.TYPE_DB2%>";
	var TYPE_SYBASE = "<%=ResCase.TYPE_SYBASE%>";
	var TYPE_INFORMIX = "<%=ResCase.TYPE_INFORMIX%>";
	var TYPE_MS_SQL2000 = "<%=ResCase.TYPE_MS_SQL2000%>";
	
	function $(id) {
		return document.getElementById(id);
	}
	
	function chg_val(type) { 
		var SUFFIX_DB = "<%=ResCase.SUFFIX_DB%>";
	
		var domain_tr = document.getElementById("domain_tr");
		var ref_res_tr = document.getElementById("ref_res_tr");
		
		if (domain_tr == null){ //没有关于域的编辑页 
			return ;
		}
		
		if (type != null && type.indexOf(SUFFIX_DB) != -1) {
			domain_tr.style.display = "";
			ref_res_tr.style.display = "";
			return;
		} 
		domain_tr.style.display = "none";
		ref_res_tr.style.display = "none";
	}
	
	function chg_type(obj) {
		var img = $("type_img");
		var lab = $("type_lab");
		img.src = "/pub/img5/meta_d/res/" + obj.value + ".png";
		lab.innerHTML = obj.options[obj.selectedIndex].text;
		
		var tg = obj.value.indexOf(SUFFIX_SYSDB);
		if(tg == -1) {
			$("ref_res_ip").disabled=true;
		}else{
			$("ref_res_ip").disabled="";
		}
		
		chg_view();
	}
	
	function chg_view() {
		var type = $("type").value
		
		if (type == TYPE_MYSQL || type == TYPE_DB2 || type == TYPE_SYBASE ||
				type == TYPE_INFORMIX || type == TYPE_MS_SQL2000) {
			$("direct_mode").disabled = "disabled";
		} else {
			$("direct_mode").disabled = "";
		}
		
		if(type == TYPE_ORACLE) {
			$("service_tr").style.display="";
			$("mark_tr").style.display = "";
		} else {
			$("service_tr").style.display="none";
			$("mark_tr").style.display = "none";
		}
	}
	
	chg_view();
	
// 	function acc_list(filter){
// 		pop_view_open("/iam/meta_d/res/db/acc_list.jsp"+ filter);		
// 	}

	function chg_disabled() {
		var domain_name = document.getElementById("domain_name");
		var auth_exp = document.getElementById("auth_exp");
		var base_dn = document.getElementById("base_dn");
		var winmode = document.getElementById("winmode");
		if(winmode.checked) {
			domain_name.disabled = "";
			auth_exp.disabled = "";
			base_dn.disabled = "";
			return;
		}
		domain_name.disabled = "disabled";
		auth_exp.disabled = "disabled";
		base_dn.disabled = "disabled";
	}
	
	function save(){
		var SUFFIX_SYSDB ="<%=ResCase.SUFFIX_SYSDB%>";
		
// 		if(fm.type.value.indexOf(SUFFIX_SYSDB) != -1){
// 			if(fm.ref_res_ip.value.length == 0){
// 				alert("请选择归属主机");
// 				return;
// 			}
// 		}
		
		if(!verify() || !confirmApp()){
			return ;
		}
		
		select_options(fm.app_svr_acc_rdn_set);
		
		fm.action="/iam/db.do?method=save";	
		
		pop_view_open("/iam/wait.jsp");
		fm.submit();
	}
		
	function go_acclist(){
		var status = fm.status.value;
		
		var acc_filter = '?host_rdn=' + '<%=tag.v("rdn")%>' ;
        	acc_filter += '&type=' + '<%=tag.v("type")%>' ;
		acc_filter += '&name=' + '<%=tag.v("name")%>' ;
		acc_filter += '&ip=' + '<%=tag.v("ip")%>' ;
		acc_filter += '&status=' + status ;
		
		pop_view_open("/iam/meta_d/res/db/acc_list.jsp" + acc_filter );		
	}
</script>
<script type="text/javascript">	
	function ref_host_sel_dlg(){
		var res_type = document.getElementById("type").value;
	
		if(fm.ref_res_ip.disabled == true){
			alert("归属主机不可更改！");
			return;
		}
		
		if (window.ActiveXObject) {
			var rv = show_dlg("归属主机", "/meta_d/res/db/dlg/ref_host_sel.jsp",
			"dialogWidth:950px;dialogHeight:390px;");
			
			if(rv == null || rv.length == 0){
				return ;		
			}	
			
			var ary = rv.split("@");
			fm.ref_res_ip.value = ary[0];
			fm.res_rdn.value = ary[1];
			fm.ref_res.value = ary[2];
			
		} else {
			retValue = window.open("/iam/meta_d/res/db/dlg/ref_host_sel.jsp","newwindow",'height=390px,width=900px,top=200px,left=400px,toolbar=no,menubar=no,scrollbars=no, resizable=no,location=no, status=no');  
		}
	}
	function app_sel_dlg(){
		if (window.ActiveXObject) {
			var rv = show_dlg("应用发布选择", "/meta_d/res/db/dlg/app_sel.jsp" ,
			"location:yes;dialogWidth:1100px;dialogHeight:390px;status:no;directories:no;scrollbars:no;Resizable=yes;help=no;");
			if(rv == null || rv.length==0){
				return ;		
			}	
			
			for(var i=0; i<rv.length; i++){
				add_app_svr_acc(fm.app_svr_acc_rdn_set,rv[i]);
			}
		} else {
			retValue = window.open("/iam/meta_d/res/db/dlg/app_sel.jsp","newwindow",'height=390px,width=900px,top=200px,left=400px,toolbar=no,menubar=no,scrollbars=no, resizable=no,location=no, status=no');  
		}
	}
	
	function dep_sel_dlg() {
		var ary = show_dlg("归属组选择", "/meta_d/res/dlg/dep_sel.jsp?domainrdn="+domian_rdn,
				"dialogWidth:730px;dialogHeight:390px;");

		if(ary == null || ary.length==0){
			return ;		
		}	

		fm.groupRdn.value = ary[0];
		fm.groupName.value = ary[1];
	}
	
	function approval_sel(num) {
		var ary = show_dlg("指定审批人选择", "/meta_d/person/dlg/approval_sel.jsp",
				"dialogWidth:1200px;dialogHeight:600px;");
		
		if(ary == null || ary.length==0 || ary == "false" || !ary){
			return;		
		}
		
		if("first" == num){
			$("approver1").value = ary;
		}else{
			if($("approver1").value == "" || $("approver1").value.length <=0){
				alert("请先选择第一审批人");
			}
			
			if(ary == $("approver1").value){
				alert("第二审批人不能与第一审批人相同");
			}else{
				$("approver2").value = ary;
			}
		}
	}

	function add_approval(){
		var first = $("approver_first");
		var second = $("approver_second");
		var obj = $("approval_chkbox");
		if(obj.checked){
			$("approval").value = "<%=ResCase.RES_SSO_LOGIN_APPROVAL %>";
			first.style.display = "block";
			second.style.display = "block";
		}else{
			$("approval").value = "<%=ResCase.RES_SSO_LOGIN_NOT_APPROVAL %>";
			first.style.display = "none";
			second.style.display = "none";
			$("approver1").value = "";
			$("approver2").value = "";
		}
	}

	add_approval();
</script>
<script type="text/javascript">	
	function add_app_svr_acc(sel, app_svr_acc){
		//alert(app_svr_acc);
		var ary = app_svr_acc.split(":");
		
		var op = document.createElement("option"); 
		op.value = ary[1]; 
		op.text = ary[0];
		
		add_options(sel, op);
	}
	
	function add_ref_host(ref_res_ip,res_rdn, ref_res,rv){
		var ary = rv.split("@");
		ref_res_ip.value = ary[0];
		res_rdn.value = ary[1];
		ref_res.value = ary[2];
	}
	
	function del_options(sel){
		var ops=sel.options;
		for(var i=0; i<ops.length;){
			if(ops[i].selected){
				ops.remove(i);		 
			}else{
				i++;
			}
		}				
	}
	
	function select_options(sel){
		var ops=sel.options;		
		for(var i=0; i<ops.length; i++){
			ops[i].selected = true;
		}
	}
	
	function add_options(sel, op_in){		
		var ops=sel.options;
		
		for(var i=0; i<ops.length; i++){
			if(ops[i].value==op_in.value){
				return ;			 
			}
		}		
		var op=document.createElement("option"); 
		op.value=op_in.value; 
		op.text=op_in.text; 
		sel.add(op);
	}
	
	function test_ip() {
		if (!is_ip(fm.ip.value)) {
			alert("填写正确的IP");
			obj_focus(fm.ip);
			return ;
		}
		var filter = "ip=" + fm.ip.value;
		
		var post_data = "method=test_ip";
		var xml_doc=new XMLDoc("/iam/common.do");	
		xml_doc.post(post_data + "&" + filter);
		var rs=xml_doc.result();
		
		var ip_status = document.getElementById("ip_status");
		
		if(rs.rv=="ok"){	
			ip_status.style.color="green";
			ip_status.innerHTML="可达";
		}else{
	 		ip_status.style.color="red";
			ip_status.innerHTML="不可达";
		}
	}
	
	function test_port() {
		if (!is_ip(fm.ip.value)) {
			alert("填写正确的IP");
			obj_focus(fm.ip);
			return ;
		}
		
		if (!is_port(fm.port)) {
			alert("填写正确的端口");
			obj_focus(fm.port);
			return ;
		}
		
		var filter = "ip=" + fm.ip.value + "&port=" + fm.port.value;
		
		var post_data = "method=test_ip_port";
		var xml_doc=new XMLDoc("/iam/common.do");	
		xml_doc.post(post_data + "&" +filter);
		var rs=xml_doc.result();
		
		var port_status = document.getElementById("port_status");
		
		if(rs.rv=="ok"){	
			port_status.style.color="green";
			port_status.innerHTML="可达";
		}else{
			port_status.style.color="red";
			port_status.innerHTML="不可达";
		}
	}
	
	function chg_start_mode(show_issuers) {
		var issuers = document.getElementById("tr_issuers");
		
		if (show_issuers) {
			issuers.style.display = "";
		} else {
			issuers.style.display = "none";
		}
	}
	
	function text_change(obj) {
		var db_service_name = $("db_service_name");
		var db_service_name_lab = $("db_service_name_lab");
		var db_name_lab = $("db_name_lab");
		var markchked = $("markchked");
		
		if(obj.checked) {
			db_service_name.disabled=false;
			db_service_name_lab.style.display = "";
			db_name_lab.style.display = "none";
			markchked.value = MARK_SERVICE_CHKED;
		} else {
			db_service_name.disabled = true;
			db_service_name_lab.style.display = "none";
			db_name_lab.style.display = "";
			markchked.value = MARK_SERVICE_UNCHKED;
		}
	}
	
	function ref_host_clean(){
		fm.ref_res_ip.value="";
		fm.res_rdn.value="";
		fm.ref_res.value="";
	}
	
	/*
	function list_insert(tab_id) {
		if(!verity_insert()){
			return ;
		}
		
		var tab = document.getElementById(tab_id);
		var tr = tab.insertRow(-1);
		tr.className="rules";
		var cell = tr.insertCell(-1);
		cell.innerHTML = tr.rowIndex; //序号
		cell.align="center";
		cell.style.borderBottom="1px solid #d3d3d3";
		
		var data = crt_data();
		
		for (var i = 0; i < data.length; i++) {
			var cell = tr.insertCell(-1);
			
			cell.innerHTML = data[i];
			cell.align="left";
			cell.style.borderLeft="1px solid #d3d3d3";
			cell.style.borderBottom="1px solid #d3d3d3";
			cell.style.paddingLeft="10px";
			
			if (i == (data.length - 1)) {
				cell.style.textAlign="center";
				cell.style.paddingTop="3px";
				cell.style.paddingLeft="0px";
			} 
		}
	}
	
	function crt_data() {
		var act_v = document.getElementById("act_v");
		var rule_v = document.getElementById("rule_v");
		
		var rv = new Array();
		rv[0] = get_v(act_v);
		rv[1] = get_v(rule_v);
		
		var rule = rv[0] + "=" + rv[1];
		
		var html = "<img style='cursor:hand;' title='上移' src='/pub/img5/secpol/up_16.png' onclick='sort_up(\"rules_v\")'/>&nbsp;";
		html += "<img style='cursor:hand;' title='下移' src='/pub/img5/secpol/down_16.png' onclick='sort_down(\"rules_v\")'/>&nbsp;";
		html += "<img style='cursor:hand;' title='删除' src='/pub/img5/secpol/del_16.png'  onclick='delete_rule(\"rules_v\",this)'/>" ;
		html += "<input type='hidden' name='sso_rules' value='" + rule + "' />";
		rv[2] = html;
		
		return rv;
	}
	
	function get_v(obj){
		if(obj == null) return "&nbsp;";
		return obj.value;
	}
	
	function getParentByTagName(obj, tagName) {
		while(true) {
			if (obj == null || obj.tagName == tagName.toUpperCase()) {
				return obj;
			}
			obj = obj.parentNode;
		}
	}
	
	function down_change() {
		var rules_desc = document.getElementById("rules_desc");
		var sso_rules = document.getElementsByName("sso_rules");
		var sso_rules_value="";
		for (var i=0; i<sso_rules.length; i++) {
			sso_rules_value += sso_rules[i].value + ",";
		}
		
		rules_desc.value = sso_rules_value.substring(0, sso_rules_value.length - 1);
	}
	
	function up_change() {
		var rules_desc = document.getElementById("rules_desc").value;
		
		var tab = document.getElementById("rules_v");
		
		if (rules_desc.length==0) {
			alert("请输入需要转换的内容.");
			return;
		}
		
		delete_all_rule("rules_v");
		
		var datas = rules_desc.split(",");
		
		for(var i=0 ; i<datas.length; i++){
			var data = datas[i];
			var index = data.indexOf("=");
			
			if(index == -1){
				return;
			} 
			
			var act_v = data.substring(0,index);
			var rule_v = data.substring(index + 1, data.length);
			w_data_one(tab, act_v, rule_v);
		}
	}
	
	//删除当前行规则
	function delete_rule(tab,obj){
		var tr = getParentTag(obj, "tr") ;
		var index = tr.rowIndex;
	    var table = document.getElementById(tab);
	    table.deleteRow(index);
	    recount(table);
	}
	
	//重新对表格进行排序
	function recount(tab){
		var rows=tab.rows;
		row_index=1;
		for(var i=2;i<rows.length;i++){
			if (rows[i].cells[0] == null) {
				continue;
			}
			rows[i].cells[0].innerHTML=row_index;
			row_index++;
		}
	}
	
	//删除全部行
	function delete_all_rule(tab){
		var table = document.getElementById(tab);
		while(table.rows.length>1){
			table.deleteRow(1);
		}
	}
	
	function sort_up(tid) {
		var e = event || window.event;	
		var src = e.target || e.srcElement;
		
		var tr = getParentByTagName(src, "tr");
				
		var src_index = tr.rowIndex + 1;
		var dst_index = src_index - 1;
		
		if (dst_index < 2) {
			return;
		}
		
		var tab = document.getElementById(tid);
		var new_row = tab.moveRow(dst_index, dst_index - 1);
		
		var nodes = tab.getElementsByTagName("tr");
		var tmp_tr = nodes[dst_index];
		
		var tmp_num0 = tmp_tr.getElementsByTagName("td")[0].innerHTML;
		tmp_tr.getElementsByTagName("td")[0].innerHTML =  new_row.getElementsByTagName("td")[0].innerHTML;
		new_row.getElementsByTagName("td")[0].innerHTML = tmp_num0;
	}
	
	function sort_down(tid) {
		var e = event || window.event;	
		var src = e.target || e.srcElement;
		
		var tr = getParentByTagName(src, "tr");
		
		var src_index = tr.rowIndex + 1;
		var dst_index = src_index - 1;
		
		var tab = document.getElementById(tid);
		var nodes = tab.getElementsByTagName("tr");
		
		if (dst_index >= nodes.length - 1) {
			return;
		}
		var new_row = tab.moveRow(dst_index, dst_index + 1);
		
		var tmp_tr = nodes[dst_index];
		
		var tmp_num0 = tmp_tr.getElementsByTagName("td")[0].innerHTML;
		tmp_tr.getElementsByTagName("td")[0].innerHTML = new_row.getElementsByTagName("td")[0].innerHTML;
		new_row.getElementsByTagName("td")[0].innerHTML = tmp_num0;
	}
	
	function w_data_one(tab, act_v, rule_v){
		var tr = tab.insertRow(-1);
		tr.className="rules";
		var cell = tr.insertCell(-1);
		cell.innerHTML = tr.rowIndex; //序号
		cell.align="center";
		cell.style.borderBottom="1px solid #d3d3d3";
		
		var data = change_crt_data(act_v, rule_v);
		
		for (var i = 0; i < data.length; i++) {
			var cell = tr.insertCell(-1);
			
			cell.innerHTML = data[i];
			cell.align="left";
			cell.style.borderLeft="1px solid #d3d3d3";
			cell.style.borderBottom="1px solid #d3d3d3";
			cell.style.paddingLeft="10px";
			
			if (i == (data.length - 1)) {
				cell.style.textAlign="center";
				cell.style.paddingTop="3px";
				cell.style.paddingLeft="0px";
			} 
		}
	}
	
	function change_crt_data(act_v, rule_v) {
		var rv = new Array();
		rv[0] = act_v;
		rv[1] = rule_v;
		
		var rule = rv[0] + "=" + rv[1];
		
		var html = "<img style='cursor:hand;' title='上移' src='/pub/img5/secpol/up_16.png' onclick='sort_up(\"rules_v\")'/>&nbsp;";
		html += "<img style='cursor:hand;' title='下移' src='/pub/img5/secpol/down_16.png' onclick='sort_down(\"rules_v\")'/>&nbsp;";
		html += "<img style='cursor:hand;' title='删除' src='/pub/img5/secpol/del_16.png'  onclick='delete_rule(\"rules_v\",this)'/>" ;
		html += "<input type='hidden' name='sso_rules' value='" + rule + "' />";
		rv[2] = html;
		
		return rv;
	}
	*/
</script>
</body>
</html>
