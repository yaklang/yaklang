# Gradle wrapper parsing regression

`GradleWrapperMain.class` is `org/gradle/wrapper/GradleWrapperMain.class` from the Gradle wrapper JAR shipped in the spring-petclinic checkout used for code-scan validation. Its bundled license is retained in `GradleWrapper-LICENSE.txt`.

SHA-256: `f02e3739c319aa17d83581478d4904d05a67b4cdef76a30f12c80cd12fe55d83`.

The optimized class reuses reference parameter slots for unrelated locals. Before javajive PR #10, lambda capture substitution produced invalid assignment targets such as `new WrapperExecutor(...) = new PathAssembler(...)`. The test guards strict Yaklang AST parsing; it does not assert full Java recompilation or behavioral equivalence of the decompiled wrapper.
