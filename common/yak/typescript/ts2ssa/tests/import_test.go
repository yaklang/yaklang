package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TODO 已知问题 当一个value通过CrateMemberCallVariable后被赋值到成员变量 后续这个value的ReplaceValue无法生效
// TODO 已知问题 当一个BluePrint触发LazyBuild时使用一个UndefinedValue 后续这个Value被Replace在蓝图中注册的StaticMember不生效(TestNamedImportWithGlobalVar)

func TestBasicImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/utils.ts", `
export function getValue(): number {
	return 42;
}

export const message = "Hello World";
const PI = 3.14
`)
	vf.AddFile("src/main.ts", `
import { getValue, message } from './utils';

console.log(getValue());
console.log(message);
console.log(PI);
function go0p(){
console.log(getValue())
}
go0p()
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $result)
	`, map[string][]string{
		"result": {"Undefined-PI", "42", "\"Hello World\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestBasicImportReverse(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { getValue, message } from './utils';

console.log(getValue());
console.log(message);
console.log(PI);
function go0p(){
console.log(getValue())
}
go0p()
`)
	vf.AddFile("src/utils.ts", `
export function getValue(): number {
	return 42;
}

export const message = "Hello World";
const PI = 3.14
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $result)
	`, map[string][]string{
		"result": {"Undefined-PI", "42", "\"Hello World\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestDefaultImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/app.ts", `
import calculate from './math';

const result = calculate();
console.log(result);
`)
	vf.AddFile("src/math.ts", `
export default function calculate(): number {
	return 100;
}
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $output)
	`, map[string][]string{
		"output": {"100"},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestNamespaceImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/index.ts", `
import * as Constants from './constants';

console.log(Constants.PI);
console.log(Constants.getVersion());
`)
	vf.AddFile("src/constants.ts", `
export const PI = 3.14;
export const E = 2.71;
export function getVersion(): string {
	return "1.0.0";
}
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $values)
	`, map[string][]string{
		"values": {"3.14", "\"1.0.0\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestMixedImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/lib.ts", `
export default class Calculator {
	static add(a: number, b: number): number {
		return a + b;
	}
}

export const version = "2.0";
export function helper(): string {
	return "helper";
}
`)
	vf.AddFile("src/main.ts", `
import Calculator, { version, helper } from './lib';

const sum = Calculator.add(10, 20);
console.log(sum);
console.log(version);
console.log(helper());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $outputs)
	`, map[string][]string{
		"outputs": {"20", "10", "\"2.0\"", "\"helper\""},
	}, true, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestRelativeImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/components/Button.ts", `
export interface ButtonProps {
	text: string;
	onClick: () => void;
}

export class Button {
	constructor(private props: ButtonProps) {}
	
	getText(): string {
		return this.props.text;
	}
}
`)
	vf.AddFile("src/pages/Home.ts", `
import { Button, ButtonProps } from '../components/Button';

const props: ButtonProps = {
	text: "Click me",
	onClick: () => console.log("clicked")
};

const button = new Button(props);
console.log(button.getText());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $text)
	`, map[string][]string{
		"text": {"\"Click me\"", "\"clicked\""},
	}, true, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestImportWithAlias(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/database.ts", `
export function connect(): string {
	return "connected";
}

export function disconnect(): string {
	return "disconnected";
}
`)
	vf.AddFile("src/service.ts", `
import { connect as dbConnect, disconnect as dbDisconnect } from './database';

const status1 = dbConnect();
const status2 = dbDisconnect();
console.log(status1);
console.log(status2);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $statuses)
	`, map[string][]string{
		"statuses": {"\"connected\"", "\"disconnected\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestImportSourceCodeRange(t *testing.T) {
	code := `
import { readFile } from 'fs';
import path from 'path';

function processFile(filename: string): void {
	const fullPath = path.join(__dirname, filename);
	readFile(fullPath, 'utf8', (err, data) => {
		if (err) throw err;
		console.log(data);
	});
}

processFile('test.txt');
`

	ssatest.CheckSyntaxFlowSource(t, code, `
		readFile as $readFileFunc
		path as $pathModule
	`, map[string][]string{
		"readFileFunc": {"readFile"},
		"pathModule":   {"path"},
	}, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestImportTypeOnly(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/types.ts", `
export interface User {
	id: number;
	name: string;
}

export type Status = 'active' | 'inactive';
`)
	vf.AddFile("src/user.ts", `
import type { User, Status } from './types';

function createUser(name: string): User {
	return {
		id: 1,
		name: name
	};
}

const user = createUser("Alice");
console.log(user.name);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $userName)
	`, map[string][]string{
		"userName": {"\"Alice\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS),
	)
}

func TestImportWithInterface(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/interfaces/IService.ts", `
export interface IService {
	getData(): Promise<string>;
	processData(data: string): string;
}
`)
	vf.AddFile("src/services/DataService.ts", `
import { IService } from '../interfaces/IService';

export class DataService implements IService {
	async getData(): Promise<string> {
		return "sample data";
	}
	
	processData(data: string): string {
		return data.toUpperCase();
	}
}
`)
	vf.AddFile("src/main.ts", `
import { DataService } from './services/DataService';

const service = new DataService();
service.getData().then(data => {
	const processed = service.processData(data);
	console.log(processed);
});
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $processed)
	`, map[string][]string{
		"processed": {"sample data"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestImportClass(t *testing.T) {
	t.Run("import class with static method", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/utils/MathUtils.ts", `
export class MathUtils {
	static PI = 3.14159;
	
	static multiply(a: number, b: number): number {
		return a * b;
	}
}
`)
		vf.AddFile("src/calculator.ts", `
import { MathUtils } from './utils/MathUtils';

const result = MathUtils.multiply(5, 10);
console.log(result);
console.log(MathUtils.PI);
`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
			console.log(* #-> as $values)
		`, map[string][]string{
			"values": {"5", "10", "3.14159"},
		}, false, ssaapi.WithLanguage(ssaconfig.TS))
	})

	t.Run("import class with instance method", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("src/models/User.ts", `
export class User {
	constructor(public name: string, public age: number) {}
	
	getInfo(): string {
		return this.name + " is " + this.age + " years old";
	}
}
`)
		vf.AddFile("src/app.ts", `
import { User } from './models/User';

const user = new User("John", 25);
console.log(user.getInfo());
`)

		ssatest.CheckSyntaxFlowWithFS(t, vf, `
			console.log(* #-> as $info)
		`, map[string][]string{
			"info": {"\" is \"", "\" years old\"", "\"John\"", "25"},
		}, true, ssaapi.WithLanguage(ssaconfig.TS))
	})
}

func TestImportEnum(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { Color, Status } from './enums/Color';

console.log(Color.Red);
console.log(Status.Approved);
`)
	vf.AddFile("src/enums/Color.ts", `
export enum Color {
	Red = "red",
	Green = "green",
	Blue = "blue"
}

export enum Status {
	Pending = 1,
	Approved = 2,
	Rejected = 3
}
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"\"red\"", "2"},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestImportEnumRecursive(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { Color, Status } from './enums/colors';

console.log(Color.Red);
console.log(Status.Approved);
`)
	vf.AddFile("src/enums/colors.ts", `
import { Palette } from './palette';

export enum Color {
 Red = Palette.SlotA,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}

export enum Status {
 Pending = 1,
 Approved = 2,
 Rejected = 3
}
`)
	vf.AddFile("src/enums/palette.ts", `
export const enum Palette {
 SlotA = "red"
}
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"\"red\"", "2"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestImportEnumRecursiveReverse(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/enums/palette.ts", `
export const enum Palette {
 SlotA = "red"
}
`)
	vf.AddFile("src/enums/colors.ts", `
import { Palette } from './palette';

export enum Color {
 Red = Palette.SlotA,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}

export enum Status {
 Pending = 1,
 Approved = 2,
 Rejected = 3
}
`)
	vf.AddFile("src/main.ts", `
import { Color, Status } from './enums/colors';

console.log(Color.Red);
console.log(Status.Approved);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"\"red\"", "2"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestNamedImportWithGlobalVar(t *testing.T) {
	t.Skip("TODO 已知问题 当一个BluePrint触发LazyBuild时使用一个UndefinedValue 后续这个Value被Replace在蓝图中注册的StaticMember不生效")
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/enums/colors.ts", `
import { PI } from './const';

export enum Color {
 Red = PI,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}
`)
	vf.AddFile("src/main.ts", `
import { Color} from './enums/colors';

console.log(Color.Red);
`)

	vf.AddFile("src/enums/const.ts", `
export const PI = 3.14
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"3.14"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestNameSpaceImportWithGlobalVar(t *testing.T) {
	t.Skip("TODO 已知问题 当一个value通过CrateMemberCallVariable后被赋值到成员变量 后续这个value的ReplaceValue无法生效")
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/enums/colors.ts", `
import * as C from './const';

export enum Color {
 Red = C.PI,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}
`)
	vf.AddFile("src/main.ts", `
import { Color } from './enums/colors';

console.log(Color.Red);
`)
	vf.AddFile("src/enums/const.ts", `
export const PI = 3.14
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"3.14"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestDefaultImportWithGlobalVar(t *testing.T) {
	t.Skip("TODO 已知问题 当一个value通过CrateMemberCallVariable后被赋值到成员变量 后续这个value的ReplaceValue无法生效")
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/enums/colors.ts", `
import GG from './const';

export enum Color {
 Red = GG,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}
`)
	vf.AddFile("src/enums/const.ts", `
const PI = 3.14
export default PI;
`)
	vf.AddFile("src/main.ts", `
import {Color} from './enums/colors';

console.log(Color.Red);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"3.14"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS), ssaapi.WithASTOrder(ssareducer.Order))
}

func TestRenamedImportWithGlobalVar(t *testing.T) {
	t.Skip("TODO 已知问题 当一个BluePrint触发LazyBuild时使用一个UndefinedValue 后续这个Value被Replace在蓝图中注册的StaticMember不生效")
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { Color} from './enums/colors';

console.log(Color.Red);
`)
	vf.AddFile("src/enums/colors.ts", `
import { PI as MyPI } from './const';

export enum Color {
 Red = MyPI,   // 二层递归来源
 Green = "green",
 Blue = "blue",
}
`)
	vf.AddFile("src/enums/const.ts", `
export const PI = 3.14
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $enumValues)
	`, map[string][]string{
		"enumValues": {"3.14"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestReExportNamed(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/lib.ts", `
export const value = 7;
export function twice(n: number) { return n; }
`)
	vf.AddFile("src/index.ts", `
export { value as answer, twice as double } from './lib';
`)
	vf.AddFile("src/main.ts", `
import { answer, double } from './index';

console.log(answer);
console.log(double(5));
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $out)
	`, map[string][]string{
		"out": {"7", "5"},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestReExportDefaultAsNamed(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/lib.ts", `
export default class Calc {
	static add(a: number) { return a+1; }
}
export const version = "3.0";
`)
	vf.AddFile("src/index.ts", `
export { default as Calc, version } from './lib';
`)
	vf.AddFile("src/main.ts", `
import { Calc, version } from './index';

console.log(Calc.add(2));
console.log(version);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $out)
	`, map[string][]string{
		"out": {"1", "2", "\"3.0\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestReExportDefaultAsNamedReverseOrder(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { Calc, version } from './index';

console.log(Calc.add(2));
console.log(version);
`)
	vf.AddFile("src/lib.ts", `
export default class Calc {
	static add(a: number) { return a+1; }
}
export const version = "3.0";
`)
	vf.AddFile("src/index.ts", `
export { default as Calc, version } from './lib';
`)
	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $out)
	`, map[string][]string{
		"out": {"1", "2", "\"3.0\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestTypeOnlyReExport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/types.ts", `
export interface User { id: number; name: string; }
`)
	vf.AddFile("src/index.ts", `
export type { User } from './types';
`)
	vf.AddFile("src/main.ts", `
import type { User } from './index';

const u: User = { id: 1, name: "Alice" };
console.log(u.name);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $name)
	`, map[string][]string{
		"name": {"\"Alice\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestExportEqualsAndImportEquals(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/math.ts", `
function sum(a: number, b: number) { return a + b + 1; }
export = sum;
`)
	vf.AddFile("src/main.ts", `
// TS ImportEquals 语法
import sum = require('./math');
console.log(sum(2, 3));
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $out)
	`, map[string][]string{
		"out": {"2", "3", "1"},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

// sanity check
func TestCircularImportBasic(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/a.ts", `
import { bVal } from './b';
export const aVal = "A" + (bVal || "");
`)
	vf.AddFile("src/b.ts", `
import { aVal } from './a';
export const bVal = "B" + (aVal ? "" : "");
`)
	vf.AddFile("src/main.ts", `
import { aVal } from './a';
import { bVal } from './b';
console.log(aVal);
console.log(bVal);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $vals)
	`, map[string][]string{
		"vals": {"\"A\"", "\"B\""},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestExportStarVsDefault(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/lib.ts", `
export default function f() { return "D"; }
export const X = "X";
`)
	vf.AddFile("src/index.ts", `
// 星号只转发命名导出，不含 default
export * from './lib';
// 想拿到 default，必须显式取别名
export { default as DF } from './lib';
`)
	vf.AddFile("src/main.ts", `
import { X, DF } from './index';
console.log(X);
console.log(DF());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $vals)
	`, map[string][]string{
		"vals": {"\"X\"", "\"D\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

func TestExportStarVsDefaultForDebug(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/index.ts", `
// 星号只转发命名导出，不含 default
export * from './lib';
`)
	vf.AddFile("src/lib.ts", `
export const X = "X";
`)
	vf.AddFile("src/main.ts", `
import { X } from './index';
console.log(X);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $vals)
	`, map[string][]string{
		"vals": {"\"X\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestDeepReExportChain 测试深层递归重导出链
// 验证递归函数能正确处理多层重导出 (A -> B -> C -> D)
func TestDeepReExportChain(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/d.ts", `
export const deepValue = 42;
export function deepFunc() { return "deep"; }
`)
	vf.AddFile("src/c.ts", `
// C 从 D 重导出
export { deepValue as cValue, deepFunc as cFunc } from './d';
`)
	vf.AddFile("src/b.ts", `
// B 从 C 重导出
export { cValue as bValue, cFunc as bFunc } from './c';
`)
	vf.AddFile("src/a.ts", `
// A 从 B 重导出
export { bValue as aValue, bFunc as aFunc } from './b';
`)
	vf.AddFile("src/main.ts", `
import { aValue, aFunc } from './a';

console.log(aValue);
console.log(aFunc());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $results)
	`, map[string][]string{
		"results": {"42", "\"deep\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestReExportWithWildcard 测试通配符重导出的递归处理
func TestReExportWithWildcard(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main.ts", `
import { alpha, beta, extra } from './top';

console.log(alpha);
console.log(beta);
console.log(extra);
`)
	vf.AddFile("src/base.ts", `
export const alpha = "alpha";
export const beta = "beta";
export const gamma = "gamma";
`)
	vf.AddFile("src/middle.ts", `
// 通配符重导出所有命名导出
export * from './base';
export const extra = "extra";
`)
	vf.AddFile("src/top.ts", `
// 再次通配符重导出
export * from './middle';
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $values)
	`, map[string][]string{
		"values": {"\"alpha\"", "\"beta\"", "\"extra\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestReExportNamespaceExport 测试命名空间重导出
func TestReExportNamespaceExport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/utils.ts", `
export const PI = 3.14;
export const E = 2.71;
export function calculate() { return 100; }
`)
	vf.AddFile("src/index.ts", `
// 命名空间重导出: export * as ns from 'module'
export * as Utils from './utils';
`)
	vf.AddFile("src/main.ts", `
import { Utils } from './index';

console.log(Utils.PI);
console.log(Utils.E);
console.log(Utils.calculate());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $values)
	`, map[string][]string{
		"values": {"3.14", "2.71", "100"},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestMixedReExportChain 测试混合类型的重导出链
// 包含命名重导出、通配符重导出和重命名
func TestMixedReExportChain(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/core.ts", `
export const coreValue = 999;
export default function coreFunc() { return "core"; }
export class CoreClass {
	static method() { return "method"; }
}
`)
	vf.AddFile("src/layer1.ts", `
// 混合：命名导出、默认导出重命名、通配符
export * from './core';
export { default as defaultCore } from './core';
`)
	vf.AddFile("src/layer2.ts", `
// 继续重导出，并添加新的导出
export { coreValue as finalValue, CoreClass, defaultCore } from './layer1';
export const layer2Value = "layer2";
`)
	vf.AddFile("src/main.ts", `
import { finalValue, CoreClass, defaultCore, layer2Value } from './layer2';

console.log(CoreClass.method());
console.log(finalValue);
console.log(defaultCore());
console.log(layer2Value);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $all)
	`, map[string][]string{
		"all": {"999", "\"method\"", "\"core\"", "\"layer2\""},
	}, true, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestWildcardReExportRecursive 测试通配符重导出的递归场景
// A 使用 B 的通配符重导出，B 又使用了 C 的通配符重导出
func TestWildcardReExportRecursive(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/c.ts", `
export const valueC1 = "c1";
export const valueC2 = "c2";
export function funcC() { return "funcC"; }
`)
	vf.AddFile("src/b.ts", `
// B 通过通配符从 C 重导出
export * from './c';
export const valueB = "b";
`)
	vf.AddFile("src/a.ts", `
// A 通过通配符从 B 重导出（B 本身也是通过通配符从 C 重导出）
export * from './b';
export const valueA = "a";
`)
	vf.AddFile("src/main.ts", `
import { valueC1, valueC2, funcC, valueB, valueA } from './a';

console.log(valueC1);
console.log(valueC2);
console.log(funcC());
console.log(valueB);
console.log(valueA);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $values)
	`, map[string][]string{
		"values": {"\"c1\"", "\"c2\"", "\"funcC\"", "\"b\"", "\"a\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}

// TestWildcardWithNamedReExportRecursive 测试通配符和命名重导出混合的递归场景
func TestWildcardWithNamedReExportRecursive(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/base.ts", `
export const x = 1;
export const y = 2;
export const z = 3;
`)
	vf.AddFile("src/middle.ts", `
// 通配符重导出 base 的所有内容
export * from './base';
// 同时添加新的命名导出
export const middle = "m";
`)
	vf.AddFile("src/top.ts", `
// 命名重导出 middle 中的部分内容（这些内容来自 base 的通配符重导出）
export { x as topX, middle as topMiddle } from './middle';
// 通配符重导出 middle 的所有内容
export * from './middle';
`)
	vf.AddFile("src/main.ts", `
import { topX, topMiddle, y, z } from './top';

console.log(topX);
console.log(y);
console.log(z);
console.log(topMiddle);
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $all)
	`, map[string][]string{
		"all": {"1", "2", "3", "\"m\""},
	}, false, ssaapi.WithLanguage(ssaconfig.TS))
}
