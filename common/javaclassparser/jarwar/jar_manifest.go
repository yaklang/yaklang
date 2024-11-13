package jarwar

import (
	"strings"
	"time"
)

type JarManifest struct {
	ManifestVersion       string
	CreatedBy             string
	BuildJdkSpec          string
	SpecificationTitle    string
	SpecificationVersion  string
	SpecificationVendor   string
	ImplementationTitle   string
	ImplementationVersion string
	ImplementationVendor  string
	AutomaticModuleName   string
	BundleDescription     string
	BundleDocURL          string
	BundleLicense         string
	BundleManifestVersion string
	BundleName            string
	BundleSCM             string
	BundleSymbolicName    string
	BundleVendor          string
	BundleVersion         string
	ExportPackage         string
	ImportPackage         string
	PrivatePackage        string
	RequireCapability     string
	BuildTime             time.Time // Added to store build time as a time.Time object
}

/*
Manifest-Version: 1.0
Created-By: Maven JAR Plugin 3.3.0
Build-Jdk-Spec: 17
Specification-Title: Maven Artifact Resolver Transport Wagon
Specification-Version: 1.9
Specification-Vendor: The Apache Software Foundation
Implementation-Title: Maven Artifact Resolver Transport Wagon
Implementation-Version: 1.9.16
Implementation-Vendor: The Apache Software Foundation
Automatic-Module-Name: org.apache.maven.resolver.transport.wagon
Bundle-Description: A transport implementation based on Maven Wagon.
Bundle-DocURL: https://maven.apache.org/resolver/maven-resolver-transport-wagon/
Bundle-License: "Apache-2.0";link="https://www.apache.org/licenses/LICENSE-2.0.txt"
Bundle-ManifestVersion: 2
Bundle-Name: Maven Artifact Resolver Transport Wagon
Bundle-SCM: url="https://github.com/apache/maven-resolver/tree/maven-resolver-1.9.16/maven-resolver-transport-wagon",connection="scm:git:https://gitbox.apache.org/repos/asf/maven-resolver.git/maven-resolver-transport-wagon",developer-connection="scm:git:https://gitbox.apache.org/repos/asf/maven-resolver.git/maven-resolver-transport-wagon",tag="maven-resolver-1.9.16"
Bundle-SymbolicName: org.apache.maven.resolver.transport.wagon
Bundle-Vendor: The Apache Software Foundation
Bundle-Version: 1.9.16
Export-Package: org.eclipse.aether.transport.wagon;uses:="javax.inject,org.apache.maven.wagon,org.eclipse.aether,org.eclipse.aether.repository,org.eclipse.aether.spi.connector.transport,org.eclipse.aether.spi.locator,org.eclipse.aether.transfer";version="1.9.16"
Import-Package: javax.inject,org.apache.maven.wagon,org.apache.maven.wagon.authentication,org.apache.maven.wagon.events,org.apache.maven.wagon.observers,org.apache.maven.wagon.proxy,org.apache.maven.wagon.repository,org.apache.maven.wagon.resource,org.codehaus.plexus,org.codehaus.plexus.classworlds.realm;version="[2.7,3)",org.codehaus.plexus.component.configurator,org.codehaus.plexus.component.configurator.converters.composite,org.codehaus.plexus.component.configurator.converters.lookup,org.codehaus.plexus.component.configurator.expression,org.codehaus.plexus.configuration,org.codehaus.plexus.configuration.xml,org.codehaus.plexus.util.xml,org.eclipse.aether;version="[1.9,2)",org.eclipse.aether.repository;version="[1.9,2)",org.eclipse.aether.spi.connector.transport;version="[1.9,2)",org.eclipse.aether.spi.locator;version="[1.9,2)",org.eclipse.aether.transfer;version="[1.9,2)",org.eclipse.aether.transport.wagon,org.eclipse.aether.util;version="[1.9,2)",org.slf4j;version="[1.7,2)"
Private-Package: org.eclipse.aether.internal.transport.wagon
Require-Capability: osgi.ee;filter:="(&(osgi.ee=JavaSE)(version=1.8))"
*/
func ParseJarManifest(manifestContent string) JarManifest {
	lines := strings.Split(manifestContent, "\n")
	jarManifest := &JarManifest{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 查找第一个冒号的位置
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			continue
		}

		key := line[:colonIndex]
		value := ""
		if len(line) > colonIndex+1 {
			value = strings.TrimSpace(line[colonIndex+1:])
		}

		switch key {
		case "Manifest-Version":
			jarManifest.ManifestVersion = value
		case "Created-By":
			jarManifest.CreatedBy = value
		case "Build-Jdk-Spec":
			jarManifest.BuildJdkSpec = value
		case "Specification-Title":
			jarManifest.SpecificationTitle = value
		case "Specification-Version":
			jarManifest.SpecificationVersion = value
		case "Specification-Vendor":
			jarManifest.SpecificationVendor = value
		case "Implementation-Title":
			jarManifest.ImplementationTitle = value
		case "Implementation-Version":
			jarManifest.ImplementationVersion = value
		case "Implementation-Vendor":
			jarManifest.ImplementationVendor = value
		case "Automatic-Module-Name":
			jarManifest.AutomaticModuleName = value
		case "Bundle-Description":
			jarManifest.BundleDescription = value
		case "Bundle-DocURL":
			jarManifest.BundleDocURL = value
		case "Bundle-License":
			jarManifest.BundleLicense = value
		case "Bundle-ManifestVersion":
			jarManifest.BundleManifestVersion = value
		case "Bundle-Name":
			jarManifest.BundleName = value
		case "Bundle-SCM":
			jarManifest.BundleSCM = value
		case "Bundle-SymbolicName":
			jarManifest.BundleSymbolicName = value
		case "Bundle-Vendor":
			jarManifest.BundleVendor = value
		case "Bundle-Version":
			jarManifest.BundleVersion = value
		case "Export-Package":
			jarManifest.ExportPackage = value
		case "Import-Package":
			jarManifest.ImportPackage = value
		case "Private-Package":
			jarManifest.PrivatePackage = value
		case "Require-Capability":
			jarManifest.RequireCapability = value
		}
	}

	return *jarManifest
}
