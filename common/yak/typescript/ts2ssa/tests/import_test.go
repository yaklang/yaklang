package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

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
		"result": {"Undefined-PI", "FreeValue-getValue", "42", "\"Hello World\""},
	}, false, ssaapi.WithLanguage(ssaapi.TS),
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
	}, false, ssaapi.WithLanguage(ssaapi.TS),
	)
}

func TestNamespaceImport(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/constants.ts", `
export const PI = 3.14;
export const E = 2.71;
export function getVersion(): string {
	return "1.0.0";
}
`)
	vf.AddFile("src/index.ts", `
import * as Constants from './constants';

console.log(Constants.PI);
console.log(Constants.getVersion());
`)

	ssatest.CheckSyntaxFlowWithFS(t, vf, `
		console.log(* #-> as $values)
	`, map[string][]string{
		"values": {"3.14", "\"1.0.0\""},
	}, false, ssaapi.WithLanguage(ssaapi.TS),
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
	}, false, ssaapi.WithLanguage(ssaapi.TS),
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
	}, true, ssaapi.WithLanguage(ssaapi.TS),
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
	}, false, ssaapi.WithLanguage(ssaapi.TS),
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
	}, ssaapi.WithLanguage(consts.TS))
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
	}, false, ssaapi.WithLanguage(ssaapi.TS),
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
	}, true, ssaapi.WithLanguage(ssaapi.TS))
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
		}, false, ssaapi.WithLanguage(ssaapi.TS))
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
		}, true, ssaapi.WithLanguage(ssaapi.TS))
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
	}, false, ssaapi.WithLanguage(ssaapi.TS))
}

// TODO: Fix environment lost in lazy build lazy build not contain full variable table context
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
	}, true, ssaapi.WithLanguage(ssaapi.TS))
}
