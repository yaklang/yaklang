package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestPythonProjectCompile_ClassInheritanceDoesNotReportLoop(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("forms.py", `
class Form:
    pass

class UserForm(Form):
    pass
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ClassMethodCallAfterInstantiation(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("admin.py", `
class Admin:
    pass

class QuokkaAdmin(Admin):
    def register(self):
        return 1

def create_admin():
    admin = QuokkaAdmin()
    return admin.register()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_LoweringSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
type UserId = int

def sink(value):
    return value

if user_id := 7:
    typed_user: UserId = user_id
    sink(typed_user)

ctx = "context"
with ctx as handle:
    sink(handle)

assert handle
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ImportTryRaiseSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
import requests as req
from pkg.sub import run as execute
from pkg.star import *

try:
    raise "boom"
except:
    req = execute
finally:
    req = execute

match req:
    case execute:
        req = run
    case _:
        req = helper
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_LegacyStmtSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
print 1

value = 3
del value

exec "value = 1" in scope, scope

def numbers():
    yield 1
    yield from items
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ForDestructuringSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
for item in [1, 2]:
    current = item

for left, right in [(1, 2), (3, 4)]:
    pair = left

for head, tail, *rest in [(1, 2, 3, 4)]:
    remain = rest
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DictLiteralNumericKeysSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
errors = {
    0: "name",
    1: "value",
}

def read_errors():
    return errors[0], errors[1]
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ClassWithoutInitCanInstantiate(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Sentinel:
    def __bool__(self):
        return False

UNSPECIFIED = Sentinel()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DynamicSelfMemberSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Store:
    def init(self):
        self.collections = {}
        self.system = "tinydb"
        return self.collections.get("index"), self.system
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_LocalImportAndDynamicCallResultSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Builder:
    def build(self):
        from pkg.factory import create
        obj = create()
        obj.parent = 1
        return obj.owner

Builder().build()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ParamIndexAndMemberSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def normalize(model, participant):
    return model["slug"], participant.email
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_LocalClassInstantiationSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def build():
    class TextFieldComparison:
        pass

    return TextFieldComparison()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_FunctionAttributeSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def build_name(method):
    def fn(self, *args, **kwargs):
        return method

    fn.__name__ = str(method)
    return fn.__name__
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DynamicSelfCallableMemberSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Document:
    def bootstrap(self):
        return self.__setup__()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_LocalInheritedClassInstantiationSmoke(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def build():
    class HeadingBlock:
        pass

    class SubHeadingBlock(HeadingBlock):
        pass

    return SubHeadingBlock()
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_KwargsPopDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def get_model_label(model):
    if isinstance(model, str):
        return model
    return model.__name__

class Field:
    def __init__(self, **kwargs):
        if "to" in kwargs.keys():
            old_to = get_model_label(kwargs.pop("to"))
        kwargs["to"] = "default.Model"

Field(related_name="test_image", to="tests.Image")
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_ChainedIndexedAttributeAssignDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Field:
    pass

fields = [Field()]
docfield = [f for f in fields if True]
docfield[0].options = "name"
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_StringKeyAssignOnCallResultDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def build():
    return []

event = build()
event["summary"] = "subject"
event["status"] = "Open"
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_NestedClassInheritanceDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Panel:
    class BoundPanel:
        pass

class CommentPanel:
    class BoundPanel(Panel.BoundPanel):
        pass

class FieldPanel:
    class BoundPanel(Panel.BoundPanel):
        pass
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DecoratedNestedClassInheritanceDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
def register_telepath_adapter(cls):
    return cls

class Component:
    pass

class Panel:
    @register_telepath_adapter
    class BoundPanel(Component):
        pass

class CommentPanel(Panel):
    class BoundPanel(Panel.BoundPanel):
        pass
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_CrossFileNestedClassInheritanceDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("base.py", `
def register_telepath_adapter(cls):
    return cls

class Component:
    pass

class Panel:
    @register_telepath_adapter
    class BoundPanel(Component):
        pass
`)
	vf.AddFile("child.py", `
from base import Panel

class CommentPanel(Panel):
    class BoundPanel(Panel.BoundPanel):
        pass
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_CrossFileNestedMetaInheritanceDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("base.py", `
class BaseImage:
    class Meta:
        abstract = True
`)
	vf.AddFile("image.py", `
from base import BaseImage

class Image(BaseImage):
    class Meta(BaseImage.Meta):
        swappable = "FILER_IMAGE_MODEL"
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DelFieldOnSliceValueDoesNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
rich_text_block = []
if hasattr(rich_text_block, "field"):
    del rich_text_block.field
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func TestPythonProjectCompile_DynamicParentAliasMembersDoNotError(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("main.py", `
class Case:
    def __init__(self):
        self.form_page = []

    def make_form_pages(self, parent=None):
        if parent is None:
            parent = self.form_page

        parent.add_child(instance=1)
        slug = f"form-{parent.locale_id}"
        return slug
`)

	requirePythonProjectCompileNoErrors(t, vf)
}

func requirePythonProjectCompileNoErrors(t *testing.T, fs *filesys.VirtualFS) {
	t.Helper()

	progs, err := ssaapi.ParseProjectWithFS(
		fs,
		ssaapi.WithLanguage(ssaconfig.PYTHON),
		ssaapi.WithMemory(true),
	)
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	for _, prog := range progs {
		require.Len(t, prog.GetErrors(), 0, prog.GetErrors().String())
	}
}
