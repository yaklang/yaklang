<%@ page language="java" contentType="text/html; charset=UTF-8" pageEncoding="UTF-8"%>
<%@ taglib uri="http://java.sun.com/jsp/jstl/core" prefix="c"%>

<html>
<body>
<form action="install" method="post">
	<label>
		<input type="radio" name="cacheServer" value="ehcache" onclick="selectCache(this);" <c:if test="${install.cacheServer == 'ehcache'}">checked='checked'</c:if>>
		Ehcache
	</label>
	<label>
		<input type="radio" name="cacheServer" value="memcache" onclick="selectCache(this);" <c:if test="${install.cacheServer == 'memcache'}">checked='checked'</c:if>>
		Memcache
	</label>

	<tr id="tr_memcacheIP" <c:if test="${install.cacheServer == 'ehcache'}">style='display: none;'</c:if>>
		<td class="name">Memcache缓存服务器IP：</td>
		<td><input class="form-text" name="memcacheIP" value="${install.memcacheIP}" size="50"></td>
	</tr>

	<c:if test="${error['installSystem'] != null}">
		<p class="error">${error['installSystem']}</p>
	</c:if>
</form>
</body>
</html>
