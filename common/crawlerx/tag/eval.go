package tag

const startsWithJS = `
if (typeof String.prototype.startsWith != 'function') {
	String.prototype.startsWith = function (prefix) {
		return this.slice(0, prefix.length) === prefix;
	};
}
`

const endsWithJS = `
if (!String.prototype.endsWith) {
	String.prototype.endsWith = function(search, this_len) {
		if (this_len === undefined || this_len > this.length) {
			this_len = this.length;
		}
		return this.substring(this_len - search.length, this_len) === search;
	};
}
`
