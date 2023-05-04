package extend

const GETIMGB64STR = `
()=>{
	canvas = document.createElement("canvas");
	context = canvas.getContext("2d");
	canvas.height = this.naturalHeight;
	canvas.width = this.naturalWidth;
	context.drawImage(this, 0, 0, this.naturalWidth, this.naturalHeight);
	base64Str = canvas.toDataURL();
	return base64Str;
}
`
