plugins {
    id("org.example.kotlin-library") version "1.0"
}

repositories {
    mavenCentral()
}

dependencies {
    implementation("org.example:sample:1.0")
}

val generatedOutput = layout.buildDirectory.dir("generated")
tasks.register("verifyGeneratedOutput") {
    doLast {
        println(generatedOutput.get().asFile)
    }
}
