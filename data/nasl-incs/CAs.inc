###############################################################################
# OpenVAS Include File
# $Id: CAs.inc 8810 2018-02-14 12:36:00Z cfischer $
#
# CA functions
#
# Authors:
# Michael Meyer
#
# Copyright:
# Copyright (c) 2016 Greenbone Networks GmbH
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2
# (or any later version), as published by the Free Software
# Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA 02110-1301
# USA.
#
# $Revision: 8810 $
#
###############################################################################

owner            = 0;
issuer           = 1;
issuer_unit      = 2;
common_name      = 3;
sha1_fingerprint = 4;
valid_from       = 5;
valid_to         = 6;
skp              = 7;
sig_algo         = 8;

global_var owner, issuer, issuer_unit, common_name, sha1_fingerprint, valid_from, valid_to, skp, sig_algo;

# Owner##Certificate Issuer Organization##Certificate Issuer Organizational Unit##Common Name or Certificate Name##SHA-1 Fingerprint##Valid From [GMT]##Valid To [GMT]##Signing Key Parameters##Signature Algorithm
known_CAs = make_list(
"AddTrust##AddTrust Externa­l CA Root####COMODO SSL CA##b4c66180c520bad688470ef80bb22beba8391c22##2011 Aug 23##2020 May 30####SHA-1",
"Actalis##Actalis S.p.A./03358520967####Actalis Authentication Root CA##F373B387065A28848AF2F34ACE192BDDC78E9CAC##2011 Sep 22##2030 Sep 22##4096##SHA-256",
"Amazon##Starfield Technologies, Inc.####Starfield Services Root Certificate Authority - G2##925A8F8D2C6D04E0665F596AFF22D863E8256F3F##2009 Sep 01##2037 Dec 31##2048##SHA-256",
"AS Sertifitseerimiskeskuse (SK)##AS Sertifitseerimiskeskus####EE Certification Centre Root CA##C9A8B9E755805E58E35377A725EBAFC37B27CCD7##2010 Oct 30##2030 Dec 17##2048##SHA-1",
"AS Sertifitseerimiskeskuse (SK)##AS Sertifitseerimiskeskus####Juur-SK##409D4BD917B55C27B69B64CB9822440DCD09B889##2001 Aug 30##2016 Aug 26##2048##SHA-1",
"Asseco Data Systems S.A. (previously Unizeto Certum)##Unizeto Sp. z o.o.####Certum CA##6252DC40F71143A22FDE9EF7348E064251B18118##2002 Jun 11##2027 Jun 11##2048##SHA-1",
"Asseco Data Systems S.A. (previously Unizeto Certum)##Unizeto Technologies S.A.##Certum Certification Authority##Certum Trusted Network CA##07E032E020B72C3F192F0628A2593A19A70F069E##2008 Oct 22##2029 Dec 31##2048##SHA-1",
"Asseco Data Systems S.A. (previously Unizeto Certum)##Unizeto Technologies S.A.##Certum Certification Authority##Certum Trusted Network CA 2##D3DD483E2BBF4C05E8AF10F5FA7626CFD3DC3092##2011 Oct 06##2046 Oct 06##4096##SHA-512",
"Atos##Atos####Atos TrustedRoot 2011##2BB1F53E550C1DC5F1D4E6B76A464B550602AC21##2011 Jul 07##2030 Dec 31##2048##SHA-256",
"Autoridad de Certificacion Firmaprofesional##Autoridad de Certificacion Firmaprofesional####Autoridad de Certificacion Firmaprofesional CIF A62634068##AEC5FB3FC8E1BFC4E54F03075A9AE800B7F7B6FA##2009 May 20##2030 Dec 31##4096##SHA-1",
"Buypass##Buypass AS-983163327####Buypass Class 2 CA 1##A0A1AB90C9FC847B3B1261E8977D5FD32261D3CC##2006 Oct 13##2016 Oct 13##2048##SHA-1",
"Buypass##Buypass AS-983163327####Buypass Class 2 Root CA##490A7574DE870A47FE58EEF6C76BEBC60B124099##2010 Oct 26##2040 Oct 26##4096##SHA-256",
"Buypass##Buypass AS-983163327####Buypass Class 3 Root CA##DAFAF7FA6684EC068F1450BDC7C281A5BCA96457##2010 Oct 26##2040 Oct 26##4096##SHA-256",
"CA Disig a.s.##Disig a.s.####CA Disig Root R1##8E1C74F8A620B9E58AF461FAEC2B4756511A52C6##2012 Jul 19##2042 Jul 19##4096##SHA-1",
"CA Disig a.s.##Disig a.s.####CA Disig Root R2##B561EBEAA4DEE4254B691A98A55747C234C7D971##2012 Jul 19##2042 Jul 19##4096##SHA-256",
"Camerfirma##AC Camerfirma SA CIF A82743287##http://www.chambersign.org##Chambers of Commerce Root##6E3A55A4190C195C93843CC0DB722E313061F0B1##2003 Sep 30##2037 Sep 30##2048##SHA-1",
"Camerfirma##AC Camerfirma S.A.####Chambers of Commerce Root - 2008##786A74AC76AB147F9C6A3050BA9EA87EFE9ACE3C##2008 Aug 01##2038 Jul 31##4096##SHA-1",
"Camerfirma##AC Camerfirma SA CIF A82743287##http://www.chambersign.org##Global Chambersign Root##339B6B1450249B557A01877284D9E02FC3D2D8E9##2003 Sep 30##2037 Sep 30##2048##SHA-1",
"Camerfirma##AC Camerfirma S.A.####Global Chambersign Root - 2008##4ABDEEEC950D359C89AEC752A12C5B29F6D6AA0C##2008 Aug 01##2038 Jul 31##4096##SHA-1",
"CerticÃ¡mara S.A.##Sociedad Cameral de CertificaciÃ³n Digital - CerticÃ¡mara S.A.####AC RaÃ­z CerticÃ¡mara S.A.##CBA1C5F8B0E35EB8B94512D3F934A2E90610D336##2006 Nov 27##2030 Apr 02##4096##SHA-1",
"Certinomis##Certinomis##0002 433998903##Certinomis - AutoritÃ© Racine##2E14DAEC28F0FA1E8E389A4EABEB26C00AD383C3##2008 Sep 17##2028 Sep 17##4096##SHA-1",
"Certinomis##Certinomis##0002 433998903##Certinomis - Root CA##9D70BB01A5A4A018112EF71C01B932C534E788A8##2013 Oct 21##2033 Oct 21##4096##SHA-256",
"certSIGN##certSIGN##certSIGN ROOT CA##certSIGN ROOT CA##FAB7EE36972662FB2DB02AF6BF03FDE87C4B2F9B##2006 Jul 04##2031 Jul 04##2048##SHA-1",
"China Financial Certification Authority (CFCA)##China Financial Certification Authority####CFCA EV ROOT##E2B8294B5584AB6B58C290466CAC3FB8398F8483##2012 Aug 08##2029 Dec 31##4096##SHA-256",
"China Internet Network Information Center (CNNIC)##China Internet Network Information Center####China Internet Network Information Center EV Certificates Root##4F99AA93FB2BD13726A1994ACE7FF005F2935D1E##2010 Aug 31##2030 Aug 31##2048##SHA-1",
"China Internet Network Information Center (CNNIC)##CNNIC####CNNIC ROOT##8BAF4C9B1DF02A92F7DA128EB91BACF498604B6F##2007 Apr 16##2027 Apr 16##2048##SHA-1",
"Chunghwa Telecom Corporation##Chunghwa Telecom Co., Ltd.##ePKI Root Certification Authority##ePKI Root Certification Authority##67650DF17E8E7E5B8240A4F4564BCFE23D69C6F0##2004 Dec 20##2034 Dec 20##4096##SHA-1",
"Comodo##Comodo CA Limited####AAA Certificate Services##D1EB23A46D17D68FD92564C2F1F1601764D8E349##2004 Jan 01##2028 Dec 31##2048##SHA-1",
"Comodo##AddTrust AB##AddTrust TTP Network##AddTrust Class 1 CA Root##CCAB0EA04C2301D6697BDD379FCD12EB24E3949D##2000 May 30##2020 May 30##2048##SHA-1",
"Comodo##AddTrust AB##AddTrust External TTP Network##AddTrust External CA Root##02FAF3E291435468607857694DF5E45B68851868##2000 May 30##2020 May 30##2048##SHA-1",
"Comodo##AddTrust AB##AddTrust TTP Network##AddTrust Public CA Root##2AB628485E78FBF3AD9E7910DD6BDF99722C96E5##2000 May 30##2020 May 30##2048##SHA-1",
"Comodo##AddTrust AB##AddTrust TTP Network##AddTrust Qualified CA Root##4D2378EC919539B5007F758F033B211EC54D8BCF##2000 May 30##2020 May 30##2048##SHA-1",
"Comodo##COMODO CA Limited####COMODO Certification Authority##6631BF9EF74F9EB6C9D5A60CBA6ABED1F7BDEF7B##2006 Dec 01##2029 Dec 31##2048##SHA-1",
"Comodo##COMODO CA Limited####COMODO ECC Certification Authority##9F744E9F2B4DBAEC0F312C50B6563B8E2D93C311##2008 Mar 06##2038 Jan 18##ECC##ECC",
"Comodo##COMODO CA Limited####COMODO RSA Certification Authority##AFE5D244A8D1194230FF479FE2F897BBCD7A8CB4##2010 Jan 19##2038 Jan 18##4096##SHA-384",
"Comodo##Comodo CA Limited####Secure Certificate Services##4A65D5F41DEF39B8B8904A4AD3648133CFC7A1D1##2004 Jan 01##2028 Dec 31##2048##SHA-1",
"Comodo##Comodo CA Limited####Trusted Certificate Services##E19FE30E8B84609E809B170D72A8C5BA6E1409BD##2004 Jan 01##2028 Dec 31##2048##SHA-1",
"Comodo##The USERTRUST Network####USERTrust ECC Certification Authority##D1CBCA5DB2D52A7F693B674DE5F05A1D0C957DF0##2010 Feb 01##2038 Jan 18##ECC P-384##ECC",
"Comodo##The USERTRUST Network####USERTrust RSA Certification Authority##2B8F1B57330DBBA2D07A6C51F70EE90DDAB9AD8E##2010 Feb 01##2038 Jan 18##4096##SHA-384",
"Comodo##The USERTRUST Network##http://www.usertrust.com##UTN-USERFirst-Client Authentication and Email##B172B1A56D95F91FE50287E14D37EA6A4463768A##1999 Jul 09##2019 Jul 09##2048##SHA-1",
"Comodo##The USERTRUST Network##http://www.usertrust.com##UTN-USERFirst-Hardware##0483ED3399AC3608058722EDBC5E4600E3BEF9D7##1999 Jul 09##2019 Jul 09##2048##SHA-1",
"Comodo##The USERTRUST Network##http://www.usertrust.com##UTN-USERFirst-Object##E12DFB4B41D7D9C32B30514BAC1D81D8385E2D46##1999 Jul 09##2019 Jul 09##2048##SHA-1",
"ComSign##ComSign####ComSign CA##E1A45B141A21DA1A79F41A42A961D669CD0634C1##2004 Mar 24##2029 Mar 19##2048##SHA-1",
"ComSign##ComSign####ComSign Secured CA##F9CD0E2CDA7624C18FBDF0F0ABB645B8F7FED57A##2004 Mar 24##2029 Mar 16##2048##SHA-1",
"Consorci AdministraciÃ³ Oberta de Catalunya (Consorci AOC, CATCert)##Agencia Catalana de Certificacio (NIF Q-0801176-I)##Jerarquia Entitats de Certificacio Catalanes##EC-ACC##28903A635B5280FAE6774C0B6DA7D6BAA64AF2E8##2003 Jan 07##2031 Jan 07##2048##SHA-1",
"Cybertrust Japan / JCSI##Japan Certification Services, Inc.####SecureSign RootCA11##3BC49F48F8F373A09C1EBDF85BB1C365C7D811B3##2009 Apr 08##2029 Apr 08##2048##SHA-1",
"D-TRUST##D-Trust GmbH####D-TRUST Root Class 3 CA 2 2009##58E8ABB0361533FB80F79B1B6D29D3FF8D5F00F0##2009 Nov 05##2029 Nov 05##2048##SHA-256",
"D-TRUST##D-Trust GmbH####D-TRUST Root Class 3 CA 2 EV 2009##96C91B0B95B4109842FAD0D82279FE60FAB91683##2009 Nov 05##2029 Nov 05##2048##SHA-256",
"Deutscher Sparkassen Verlag GmbH (S-TRUST, DSV-Gruppe)##Deutscher Sparkassen Verlag GmbH####S-TRUST Authentication and Encryption Root CA 2005PN##BEB5A995746B9EDF738B56E6DF437A77BE106B81##2005 Jun 22##2030 Jun 21##2048##SHA-1",
"Deutscher Sparkassen Verlag GmbH (S-TRUST, DSV-Gruppe)##Deutscher Sparkassen Verlag GmbH##S-TRUST Certification Services##S-TRUST Universal Root CA##1B3D1114EA7A0F9558544195BF6B2582AB40CE9A##2013 Oct 22##2038 Oct 21##2048##SHA-256",
"Deutscher Sparkassen Verlag GmbH (S-TRUST, DSV-Gruppe)##TC TrustCenter GmbH##TC TrustCenter Class 3 CA##TC TrustCenter Class 3 CA II##8025EFF46E70C8D472246584FE403B8A8D6ADBF5##2006 Jan 12##2025 Dec 31##2048##SHA-1",
"Dhimyotis / Certigna##Dhimyotis####Certigna##B12E13634586A46F1AB2606837582DC4ACFD9497##2007 Jun 29##2027 Jun 29##2048##SHA-1",
"DigiCert##Baltimore##CyberTrust##Baltimore CyberTrust Root##D4DE20D05E66FC53FE1A50882C78DB2852CAE474##2000 May 12##2025 May 12##2048##SHA-1",
"DigiCert##Cybertrust, Inc####Cybertrust Global Root##5F43E5B1BFF8788CAC1CC7CA4A9AC6222BCC34C6##2006 Dec 15##2021 Dec 15##2048##SHA-1",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Assured ID Root CA##0563B8630D62D75ABBC8AB1E4BDFB5A899B24D43##2006 Nov 10##2031 Nov 10##2048##SHA-1",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Assured ID Root G2##A14B48D943EE0A0E40904F3CE0A4C09193515D3F##2013 Aug 01##2038 Jan 15##2048##SHA-256",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Assured ID Root G3##F517A24F9A48C6C9F8A200269FDC0F482CAB3089##2013 Aug 01##2038 Jan 15##ECC##ECC",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Global Root CA##A8985D3A65E5E5C4B2D7D66D40C6DD2FB19C5436##2006 Nov 10##2031 Nov 10##2048##SHA-1",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Global Root G2##DF3C24F9BFD666761B268073FE06D1CC8D4F82A4##2013 Aug 01##2038 Jan 15##2048##SHA-256",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Global Root G3##7E04DE896A3E666D00E687D33FFAD93BE83D349E##2013 Aug 01##2038 Jan 15##ECC##ECC",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert High Assurance EV Root CA##5FB7EE0633E259DBAD0C4C9AE6D38F1A61C7DC25##2006 Nov 10##2031 Nov 10##2048##SHA-1",
"DigiCert##DigiCert Inc##www.digicert.com##DigiCert Trusted Root G4##DDFB16CD4931C973A2037D3FC83A4D7D775D05E4##2013 Aug 01##2038 Jan 15##4096##SHA-384",
"DocuSign (OpenTrust/Keynectis)##Certplus####Certplus Root CA G1##22FDD0B7FDA24E0DAC492CA0ACA67B6A1FE3F766##2014 May 26##2038 Jan 15##4096##SHA-512",
"DocuSign (OpenTrust/Keynectis)##Certplus####Certplus Root CA G2##4F658E1FE906D82802E9544741C954255D69CC1A##2014 May 26##2038 Jan 15##ECC P-384##ECC",
"DocuSign (OpenTrust/Keynectis)##Certplus####Class 2 Primary CA##74207441729CDD92EC7931D823108DC28192E2BB##1999 Jul 07##2019 Jul 06##2048##SHA-1",
"DocuSign (OpenTrust/Keynectis)##OpenTrust####OpenTrust Root CA G1##7991E834F7E2EEDD08950152E9552D14E958D57E##2014 May 26##2038 Jan 15##4096##SHA-256",
"DocuSign (OpenTrust/Keynectis)##OpenTrust####OpenTrust Root CA G2##795F8860C5AB7C3D92E6CBF48DE145CD11EF600B##2014 May 26##2038 Jan 15##4096##SHA-512",
"DocuSign (OpenTrust/Keynectis)##OpenTrust####OpenTrust Root CA G3##6E2664F356BF3455BFD1933F7C01DED813DA8AA6##2014 May 26##2038 Jan 15##ECC P-384##ECC",
"e-tugra##E-TuÄra EBG BiliÅim Teknolojileri ve Hizmetleri A.Å.##E-Tugra Sertifikasyon Merkezi##E-Tugra Certification Authority##51C6E70849066EF392D45CA00D6DA3628FC35239##2013 Mar 05##2023 Mar 03##4096##SHA-256",
"e-tugra##EBG BiliÅim Teknolojileri ve Hizmetleri A.Å.####EBG Elektronik Sertifika Hizmet SaÄlayÄ±cÄ±sÄ±##8C96BAEBDD2B070748EE303266A0F3986E7CAE58##2006 Aug 17##2016 Aug 14##4096##SHA-1",
"EDICOM##EDICOM##PKI##ACEDICOM Root##E0B4322EB2F6A568B654538448184A5036874384##2008 Apr 18##2028 Apr 13##4096##SHA-1",
"Entrust##AffirmTrust####AffirmTrust Commercial##F9B5B632455F9CBEEC575F80DCE96E2CC7B278B7##2010 Jan 29##2030 Dec 31##2048##SHA-256",
"Entrust##AffirmTrust####AffirmTrust Networking##293621028B20ED02F566C532D1D6ED909F45002F##2010 Jan 29##2030 Dec 31##2048##SHA-1",
"Entrust##AffirmTrust####AffirmTrust Premium##D8A6332CE0036FB185F6634F7D6A066526322827##2010 Jan 29##2040 Dec 31##4096##SHA-384",
"Entrust##AffirmTrust####AffirmTrust Premium ECC##B8236B002F1D16865301556C11A437CAEBFFC3BB##2010 Jan 29##2040 Dec 31##ECC##ECC",
"Entrust##Entrust, Inc.##(c) 2006 Entrust, Inc.##Entrust Root Certification Authority##B31EB1B740E36C8402DADC37D44DF5D4674952F9##2006 Nov 27##2026 Nov 27##2048##SHA-1",
"Entrust##Entrust, Inc.##(c) 2012 Entrust, Inc. - for authorized use only##Entrust Root Certification Authority - EC1##20D80640DF9B25F512253A11EAF7598AEB14B547##2012 Dec 18##2037 Dec 18##ECC P-384##ECC",
"Entrust##Entrust, Inc.##(c) 2009 Entrust, Inc.-for authorized use only##Entrust Root Certification Authority - G2##8CF427FD790C3AD166068DE81E57EFBB932272D4##2009 Jul 07##2030 Dec 07##2048##SHA-256",
"Entrust##Entrust.net##OU = (c) 1999 Entrust.net LimitedOU = www.entrust.net/CPS_2048 incorp. by ref. (limits liab.)##Entrust.net Certification Authority (2048)##503006091D97D4F5AE39F7CBE7927D7D652D3431##1999 Dec 24##2029 Jul 24##2048##SHA-1",
"GlobalSign##GlobalSign##GlobalSign ECC Root CA - R4##GlobalSign ECC Root CA - R4##6969562E4080F424A1E7199F14BAF3EE58AB6ABB##2012 Nov 13##2038 Jan 19##ECC P-256##ECC",
"GlobalSign##GlobalSign##GlobalSign ECC Root CA - R5##GlobalSign ECC Root CA - R5##1F24C630CDA418EF2069FFAD4FDD5F463A1B69AA##2012 Nov 13##2038 Jan 19##ECC P-384##ECC",
"GlobalSign##GlobalSign nv-sa##Root CA##GlobalSign Root CA##B1BC968BD4F49D622AA89A81F2150152A41D829C##1998 Sep 01##2028 Jan 28##2048##SHA-1",
"GlobalSign##GlobalSign##GlobalSign Root CA - R2##GlobalSign Root CA - R2##75E0ABB6138512271C04F85FDDDE38E4B7242EFE##2006 Dec 15##2021 Dec 15##2048##SHA-1",
"GlobalSign##GlobalSign##GlobalSign Root CA - R3##GlobalSign Root CA - R3##D69B561148F01C77C54578C10926DF5B856976AD##2009 Mar 18##2029 Mar 18##2048##SHA-256",
"GoDaddy##The Go Daddy Group, Inc.##Go Daddy Class 2 Certification Authority##Go Daddy Class 2 CA##2796BAE63F1801E277261BA0D77770028F20EEE4##2004 Jun 29##2034 Jun 29##2048##SHA-1",
"GoDaddy##GoDaddy.com, Inc.####Go Daddy Root Certificate Authority - G2##47BEABC922EAE80E78783462A79F45C254FDE68B##2009 Sep 01##2037 Dec 31##2048##SHA-256",
"GoDaddy##Starfield Technologies, Inc.##Starfield Class 2 Certification Authority##Starfield Class 2 CA##AD7E1C28B064EF8F6003402014C3D0E3370EB58A##2004 Jun 29##2034 Jun 29##2048##SHA-1",
"GoDaddy##Starfield Technologies, Inc.####Starfield Root Certificate Authority - G2##B51C067CEE2B0C3DF855AB2D92F4FE39D4E70F0E##2009 Sep 01##2037 Dec 31##2048##SHA-256",
"Government of France (ANSSI, DCSSI)##PM/SGDN##DCSSI (new name is ANSSI)##IGC/A##60D68974B5C2659E8A0FC1887C88D246691B182C##2002 Dec 13##2020 Oct 17##2048##SHA-1",
"Government of Hong Kong (SAR), Hongkong Post, Certizen##Hongkong Post####Hongkong Post Root CA 1##D6DAA8208D09D2154D24B52FCB346EB258B28A58##2003 May 15##2023 May 15##2048##SHA-1",
"Government of Japan, Ministry of Internal Affairs and Communications##Japanese Government##ApplicationCA##Japanese Government ApplicationCA##7F8AB0CFD051876A66F3360F47C88D8CD335FC74##2007 Dec 12##2017 Dec 12##2048##SHA-1",
"Government of Spain, Autoritat de CertificaciÃ³ de la Comunitat Valenciana (ACCV)##ACCV##PKIACCV##ACCVRAIZ1##93057A8815C64FCE882FFA9116522878BC536417##2011 May 05##2030 Dec 31##4096##SHA-1",
"Government of Spain, Autoritat de CertificaciÃ³ de la Comunitat Valenciana (ACCV)##Generalitat Valenciana##PKIGVA##Root CA Generalitat Valenciana##A073E5C5BD43610D864C21130A855857CC9CEA46##2001 Jul 06##2021 Jul 01##2048##SHA-1",
"Government of Taiwan, Government Root Certification Authority (GRCA)##Government Root Certification Authority####Taiwan Government Root Certification Authority##F48B11BFDEABBE94542071E641DE6BBE882B40B9##2002 Dec 05##2032 Dec 05##4096##SHA-1",
"Government of The Netherlands, PKIoverheid (Logius)##Staat der Nederlanden####Staat der Nederlanden EV Root CA##76E27EC14FDB82C1C0A675B505BE3D29B4EDDBBB##2010 Dec 08##2022 Dec 08##4096##SHA-256",
"Government of The Netherlands, PKIoverheid (Logius)##Staat der Nederlanden####Staat der Nederlanden Root CA - G2##59AF82799186C7B47507CBCF035746EB04DDB716##2008 Mar 26##2020 Mar 25##4096##SHA-256",
"Government of The Netherlands, PKIoverheid (Logius)##Staat der Nederlanden####Staat der Nederlanden Root CA - G3##D8EB6B41519259E0F3E78500C03DB68897C9EEFC##2013 Nov 14##2028 Nov 13##4096##SHA-256",
"Government of Turkey, Kamu Sertifikasyon Merkezi (Kamu SM)##TÃ¼rkiye Bilimsel ve Teknolojik AraÅtÄ±rma Kurumu - TÃBÄ°TAK##Kamu Sertifikasyon Merkezi##TÃBÄ°TAK UEKAE KÃ¶k Sertifika Hizmet SaÄlayÄ±cÄ±sÄ± - SÃ¼rÃ¼m 3##1B4B396126276B6491A2686DD70243212D1F1D96##2007 Aug 24##2017 Aug 21##2048##SHA-1",
"HARICA##Hellenic Academic and Research Institutions Cert. Authority####Hellenic Academic and Research Institutions ECC RootCA 2015##9FF1718D92D59AF37D7497B4BC6F84680BBAB666##2015 Jul 07##2040 Jun 30##ECC P-384##ECC",
"HARICA##Hellenic Academic and Research Institutions Cert. Authority####Hellenic Academic and Research Institutions RootCA 2011##FE45659B79035B98A161B5512EACDA580948224D##2011 Dec 06##2031 Dec 01##2048##SHA-1",
"HARICA##Hellenic Academic and Research Institutions Cert. Authority####Hellenic Academic and Research Institutions RootCA 2015##010C0695A6981914FFBF5FC6B0B695EA29E912A6##2015 Jul 07##2040 Jun 30##4096##SHA-256",
"IdenTrust##Digital Signature Trust##DST ACES##DST ACES CA X6##4054DA6F1C3F4074ACED0FECCDDB79D153FB901D##2003 Nov 20##2017 Nov 20##2048##SHA-1",
"IdenTrust##Digital Signature Trust Co.####DST Root CA X3##DAC9024F54D8F6DF94935FB1732638CA6AD77C13##2000 Sep 30##2021 Sep 30##2048##SHA-1",
"IdenTrust##IdenTrust####IdenTrust Commercial Root CA 1##DF717EAA4AD94EC9558499602D48DE5FBCF03A25##2014 Jan 16##2034 Jan 16##4096##SHA-256",
"IdenTrust##IdenTrust####IdenTrust Public Sector Root CA 1##BA29416077983FF4F3EFF231053B2EEA6D4D45FD##2014 Jan 16##2034 Jan 16##4096##SHA-256",
"Izenpe S.A.##IZENPE S.A.####Izenpe.com##2F783D255218A74A653971B52CA29C45156FE919##2007 Dec 13##2037 Dec 13##4096##SHA-256",
"Krajowa Izba Rozliczeniowa S.A. (KIR)##Krajowa Izba Rozliczeniowa S.A.####SZAFIR ROOT CA2##E252FA953FEDDB2460BD6E28F39CCCCF5EB33FDE##2015 Oct 19##2035 Oct 19##2048##SHA-256",
"Microsec e-SzignÃ³ CA##Microsec Ltd.##e-Szigno CA##Microsec e-Szigno Root CA##2388C9D371CC9E963DFF7D3CA7CEFCD625EC190D##2005 Apr 06##2017 Apr 06##2048##SHA-1",
"Microsec e-SzignÃ³ CA##Microsec Ltd.####Microsec e-Szigno Root CA 2009##89DF74FE5CF40F4A80F9E3377D54DA91E101318E##2009 Jun 16##2029 Dec 30##2048##SHA-256",
"NetLock Ltd.##NetLock Kft.##TanÃºsÃ­tvÃ¡nykiadÃ³k (Certification Services)##NetLock Arany (Class Gold) FÅtanÃºsÃ­tvÃ¡ny##06083F593F15A104A069A46BA903D006B7970991##2008 Dec 11##2028 Dec 06##2048##SHA-256",
"PROCERT##Sistema Nacional de Certificacion Electronica##Superintendencia de Servicios de Certificacion Electronica##PSCProcert##70C18D74B428810AE4FDA575D7019F99B03D5074##2010 Dec 28##2020 Dec 25##4096##SHA-256",
"QuoVadis##QuoVadis Limited####QuoVadis Root CA 1 G3##1B8EEA5796291AC939EAB80A811A7373C0937967##2012 Jan 12##2042 Jan 12##4096##SHA-256",
"QuoVadis##QuoVadis Limited####QuoVadis Root CA 2##CA3AFBCF1240364B44B216208880483919937CF7##2006 Nov 24##2031 Nov 24##4096##SHA-1",
"QuoVadis##QuoVadis Limited####QuoVadis Root CA 2 G3##093C61F38B8BDC7D55DF7538020500E125F5C836##2012 Jan 12##2042 Jan 12##4096##SHA-256",
"QuoVadis##QuoVadis Limited####QuoVadis Root CA 3##1F4914F7D874951DDDAE02C0BEFD3A2D82755185##2006 Nov 24##2031 Nov 24##4096##SHA-1",
"QuoVadis##QuoVadis Limited####QuoVadis Root CA 3 G3##4812BD923CA8C43906E7306D2796E6A4CF222E7D##2012 Jan 12##2042 Jan 12##4096##SHA-256",
"QuoVadis##QuoVadis Limited##Root Certification Authority##QuoVadis Root Certification Authority##DE3F40BD5093D39B6C60F6DABC076201008976C9##2001 Mar 19##2021 Mar 17##2048##SHA-1",
"RSA the Security Division of EMC##RSA Security Inc##RSA Security 2048 V3##RSA Security 2048 v3##25019019CFFBD9991CB76825748D945F30939542##2001 Feb 22##2026 Feb 22##2048##SHA-1",
"SECOM Trust Systems Co. Ltd.##SECOM Trust Systems CO.,LTD.##Security Communication EV RootCA1##Security Communication EV RootCA1##FEB8C432DCF9769ACEAE3DD8908FFD288665647D##2007 Jun 06##2037 Jun 06##2048##SHA-1",
"SECOM Trust Systems Co. Ltd.##SECOM Trust.net##Security Communication RootCA1##Security Communication RootCA1##36B12B49F9819ED74C9EBC380FC6568F5DACB2F7##2003 Sep 30##2023 Sep 30##2048##SHA-1",
"SECOM Trust Systems Co. Ltd.##SECOM Trust Systems CO.,LTD.##Security Communication RootCA2##Security Communication RootCA2##5F3B8CF2F810B37D78B4CEEC1919C37334B9C774##2009 May 29##2029 May 29##2048##SHA-256",
"Start Commercial (StartCom) Ltd.##StartCom Ltd.##Secure Digital Certificate Signing##StartCom Certification Authority##3E2BF7F2031B96F38CE6C4D8A85D3E2D58476A0F##2006 Sep 17##2036 Sep 17##4096##SHA-1",
"Start Commercial (StartCom) Ltd.##StartCom Ltd.##Secure Digital Certificate Signing##StartCom Certification Authority##A3F1333FE242BFCFC5D14E8F394298406810D1A0##2006 Sep 17##2036 Sep 17##4096##SHA-256",
"Start Commercial (StartCom) Ltd.##StartCom Ltd.####StartCom Certification Authority G2##31F1FD68226320EEC63B3F9DEA4A3E537C7C3917##2010 Jan 01##2039 Dec 31##4096##SHA-256",
"Swisscom (Switzerland) Ltd##Swisscom##Digital Certificate Services##Swisscom Root CA 1##5F3AFC0A8B64F686673474DF7EA9A2FEF9FA7A51##2005 Aug 18##2025 Aug 18##4096##SHA-1",
"Swisscom (Switzerland) Ltd##Swisscom##Digital Certificate Services##Swisscom Root CA 2##77474FC630E40F4C47643F84BAB8C6954A8A41EC##2011 Jun 24##2031 Jun 25##4096##SHA-256",
"Swisscom (Switzerland) Ltd##Swisscom##Digital Certificate Services##Swisscom Root EV CA 2##E7A19029D3D552DC0D0FC692D3EA880D152E1A6B##2011 Jun 24##2031 Jun 25##4096##SHA-256",
"SwissSign AG##SwissSign AG####SwissSign Gold CA - G2##D8C5388AB7301B1B6ED47AE645253A6F9F1A2761##2006 Oct 25##2036 Oct 25##4096##SHA-1",
"SwissSign AG##SwissSign AG####SwissSign Platinum CA - G2##56E0FAC03B8F18235518E5D311CAE8C24331AB66##2006 Oct 25##2036 Oct 25##4096##SHA-1",
"SwissSign AG##SwissSign AG####SwissSign Silver CA - G2##9BAAE59F56EE21CB435ABE2593DFA7F040D11DCB##2006 Oct 25##2036 Oct 25##4096##SHA-1",
"Symantec / GeoTrust##Equifax##Equifax Secure Certificate Authority##Equifax Secure Certificate Authority##D23209AD23D314232174E40D7F9D62139786633A##1998 Aug 22##2018 Aug 22##1024##SHA-1",
"Symantec / GeoTrust##Equifax Secure Inc.####Equifax Secure eBusiness CA-1##DA40188B9189A3EDEEAEDA97FE2F9DF5B7D18A41##1999 Jun 21##2020 Jun 21##1024##MD5",
"Symantec / GeoTrust##Equifax Secure Inc.####Equifax Secure Global eBusiness CA-1##7E784A101C8265CC2DE1F16D47B440CAD90A1945##1999 Jun 21##2020 Jun 21##1024##MD5",
"Symantec / GeoTrust##GeoTrust Inc.####GeoTrust Global CA##DE28F4A4FFE5B92FA3C503D1A349A7F9962A8212##2002 May 21##2022 May 21##2048##SHA-1",
"Symantec / GeoTrust##GeoTrust Inc.####GeoTrust Global CA 2##A9E9780814375888F20519B06D2B0D2B6016907D##2004 Mar 04##2019 Mar 04##2048##SHA-1",
"Symantec / GeoTrust##GeoTrust Inc.####GeoTrust Primary Certification Authority##323C118E1BF7B8B65254E2E2100DD6029037F096##2006 Nov 27##2036 Jul 16##2048##SHA-1",
"Symantec / GeoTrust##GeoTrust Inc.##(c) 2007 GeoTrust Inc. - For authorized use only##GeoTrust Primary Certification Authority - G2##8D1784D537F3037DEC70FE578B519A99E610D7B0##2007 Nov 05##2038 Jan 18##ECC##ECC",
"Symantec / GeoTrust##GeoTrust Inc.##(c) 2008 GeoTrust Inc. - For authorized use only##GeoTrust Primary Certification Authority - G3##039EEDB80BE7A03C6953893B20D2D9323A4C2AFD##2008 Apr 02##2037 Dec 01##2048##SHA-256",
"Symantec / GeoTrust##GeoTrust Inc.####GeoTrust Universal CA##E621F3354379059A4B68309D8A2F74221587EC79##2004 Mar 04##2029 Mar 04##4096##SHA-1",
"Symantec / GeoTrust##GeoTrust Inc.####GeoTrust Universal CA 2##379A197B418545350CA60369F33C2EAF474F2079##2004 Mar 04##2029 Mar 04##4096##SHA-1",
"Symantec / Thawte##thawte, Inc.##(c) 2006 thawte, Inc. - For authorized use only##thawte Primary Root CA##91C6D6EE3E8AC86384E548C299295C756C817B81##2006 Nov 17##2036 Jul 16##2048##SHA-1",
"Symantec / Thawte##thawte, Inc.##(c) 2007 thawte, Inc. - For authorized use only##thawte Primary Root CA - G2##AADBBC22238FC401A127BB38DDF41DDB089EF012##2007 Nov 05##2038 Jan 18##ECC##ECC",
"Symantec / Thawte##thawte, Inc.##(c) 2008 thawte, Inc. - For authorized use only##thawte Primary Root CA - G3##F18B538D1BE903B6A6F056435B171589CAF36BF2##2008 Apr 02##2037 Dec 01##2048##SHA-256",
"Symantec / VeriSign##VeriSign, Inc.##Class 1 Public Primary Certification Authority##VeriSign Class 1 Public PCA##CE6A64A309E42FBBD9851C453E6409EAE87D60F1##1996 Jan 29##2028 Aug 02##1024##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##(c) 1999 VeriSign, Inc. - For authorized use only##VeriSign Class 1 Public PCA - G3##204285DCF7EB764195578E136BD4B7D1E98E46A5##1999 Oct 01##2036 Jul 16##2048##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##(c) 1999 VeriSign, Inc. - For authorized use only##VeriSign Class 2 Public PCA - G3##61EF43D77FCAD46151BC98E0C35912AF9FEB6311##1999 Oct 01##2036 Jul 16##2048##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##(c) 1998 VeriSign, Inc. - For authorized use only##VeriSign Class 2 Public PCA â G2##B3EAC44776C9C81CEAF29D95B6CCA0081B67EC9D##1998 May 18##2028 Aug 01##1024##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##Class 3 Public Primary Certification Authority##VeriSign Class 3 Public PCA - MD2##742C3192E607E424EB4549542BE1BBC53E6174E2##1996 Jan 29##2028 Aug 01##1024##MD2",
"Symantec / VeriSign##VeriSign, Inc.##(c) 1999 VeriSign, Inc. - For authorized use only##VeriSign Class 3 Public Primary Certification Authority - G3##132D0D45534B6997CDB2D5C339E25576609B5CC6##1999 Oct 01##2036 Jul 16##2048##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##(c) 2007 VeriSign, Inc. - For authorized use only##VeriSign Class 3 Public Primary Certification Authority - G4##22D5D8DF8F0231D18DF79DB7CF8A2D64C93F6C3A##2007 Nov 05##2038 Jan 18##ECC##ECC",
"Symantec / VeriSign##VeriSign, Inc.##(c) 2006 VeriSign, Inc. - For authorized use only##VeriSign Class 3 Public Primary Certification Authority - G5##4EB6D578499B1CCF5F581EAD56BE3D9B6744A5E5##2006 Nov 08##2036 Jul 16##2048##SHA-1",
"Symantec / VeriSign##VeriSign, Inc.##(c) 2008 VeriSign, Inc. - For authorized use only##VeriSign Universal Root Certification Authority##3679CA35668772304D30A5FB873B0FA77BB70D54##2008 Apr 02##2037 Dec 01##2048##SHA-256",
"T-Systems International GmbH (Deutsche Telekom)##Deutsche Telekom AG##T-TeleSec Trust Center##Deutsche Telekom Root CA 2##85A408C09C193E5D51587DCDD61330FD8CDE37BF##1999 Jul 09##2019 Jul 09##2048##SHA-1",
"T-Systems International GmbH (Deutsche Telekom)##T-Systems Enterprise Services GmbH##T-Systems Trust Center##T-TeleSec GlobalRoot Class 2##590D2D7D884F402E617EA562321765CF17D894E9##2008 Oct 01##2033 Oct 01##2048##SHA-256",
"T-Systems International GmbH (Deutsche Telekom)##T-Systems Enterprise Services GmbH##T-Systems Trust Center##T-TeleSec GlobalRoot Class 3##55A6723ECBF2ECCDC3237470199D2ABE11E381D1##2008 Oct 01##2033 Oct 01##2048##SHA-256",
"Taiwan-CA Inc. (TWCA)##TAIWAN-CA##Root CA##TWCA Global Root CA##9CBB4853F6A4F6D352A4E83252556013F5ADAF65##2012 Jun 27##2030 Dec 31##4096##SHA-256",
"Taiwan-CA Inc. (TWCA)##TAIWAN-CA##Root CA##TWCA Root Certification Authority##CF9E876DD3EBFC422697A3B5A37AA076A9062348##2008 Aug 28##2030 Dec 31##2048##SHA-1",
"Telia Company (formerly TeliaSonera)##Sonera####Sonera Class2 CA##37F76DE6077C90C5B13E931AB74110B4F2E49A27##2001 Apr 06##2021 Apr 06##2048##SHA-1",
"Telia Company (formerly TeliaSonera)##TeliaSonera####TeliaSonera Root CA v1##4313BB96F1D5869BC14E6A92F6CFF63469878237##2007 Oct 18##2032 Oct 18##4096##SHA-1",
"Trustis##Trustis Limited##Trustis FPS Root CA##Trustis FPS Root CA##3BC0380B33C3F6A60C86152293D9DFF54B81C004##2003 Dec 23##2024 Jan 21##2048##SHA-1",
"Trustwave##SecureTrust Corporation####Secure Global CA##3A44735AE581901F248661461E3B9CC45FF53A1B##2006 Nov 07##2029 Dec 31##2048##SHA-1",
"Trustwave##SecureTrust Corporation####SecureTrust CA##8782C6C304353BCFD29692D2593E7D44D934FF11##2006 Nov 07##2029 Dec 31##2048##SHA-1",
"Trustwave##XRamp Security Services Inc##www.xrampsecurity.com##XRamp Global Certification Authority##B80186D1EB9C86A54104CF3054F34C52B7E558C6##2004 Nov 01##2035 Jan 01##2048##SHA-1",
"TurkTrust##TÃRKTRUST Bilgi Ä°letiÅim ve BiliÅim GÃ¼venliÄi Hizmetleri A.Å. (c) AralÄ±k 2007####TÃRKTRUST Elektronik Sertifika Hizmet SaÄlayÄ±cÄ±sÄ± 2##F17F6FB631DC99E3A3C87FFE1CF1811088D96033##2007 Dec 25##2017 Dec 22##2048##SHA-1",
"TurkTrust##TÃRKTRUST Bilgi Ä°letiÅim ve BiliÅim GÃ¼venliÄi Hizmetleri A.Å.####TÃRKTRUST Elektronik Sertifika Hizmet SaÄlayÄ±cÄ±sÄ± H5##C418F64D46D1DF003D2730137243A91211C675FB##2013 Apr 30##2023 Apr 28##2048##SHA-256",
"TurkTrust##TÃRKTRUST Bilgi Ä°letiÅim ve BiliÅim GÃ¼venliÄi Hizmetleri A.Å.####TÃRKTRUST Elektronik Sertifika Hizmet SaÄlayÄ±cÄ±sÄ± H6##8A5C8CEEA503E60556BAD81BD4F6C9B0EDE52FE0##2013 Dec 18##2023 Dec 18##2048##SHA-256",
"Visa##VISA##Visa International Service Association##Visa eCommerce Root##70179B868C00A4FA609152223F9F3E32BDE00562##2002 Jun 26##2022 Jun 24##2048##SHA-1",
"Web.com##Network Solutions L.L.C.####Network Solutions Certificate Authority##74F8A3C3EFE7B390064B83903C21646020E5DFCE##2006 Dec 01##2029 Dec 31##2048##SHA-1",
"Wells Fargo Bank N.A.##Wells Fargo WellsSecure##Wells Fargo Bank NA##WellsSecure Public Root Certificate Authority##E7B4F69D61EC9069DB7E90A7401A3CF47D4FE8EE##2007 Dec 13##2022 Dec 14##2048##SHA-1",
"WISeKey##WISeKey##OISTE Foundation Endorsed##OISTE WISeKey Global Root GA CA##5922A1E15AEA163521F898396A4646B0441B0FA9##2005 Dec 11##2037 Dec 11##2048##SHA-1",
"WISeKey##WISeKey##OISTE Foundation Endorsed##OISTE WISeKey Global Root GB CA##0FF9407618D3D76A4B98F0A8359E0CFD27ACCCED##2014 Dec 01##2039 Dec 01##2048##SHA-256",
"WoSign CA Limited##WoSign CA Limited####CA WoSign ECC Root##D27AD2BEED94C0A13CC72521EA5D71BE8119F32B##2014 Nov 08##2044 Nov 08##ECC P-384##ECC",
"WoSign CA Limited##WoSign CA Limited####CA##1632478D89F9213A92008563F5A4A7D312408AD6##2009 Aug 08##2039 Aug 08##4096##SHA-256",
"WoSign CA Limited##WoSign CA Limited####Certification Authority of WoSign##B94294BF91EA8FB64BE61097C7FB001359B676CB##2009 Aug 08##2039 Aug 08##4096##SHA-1",
"WoSign CA Limited##WoSign CA Limited####Certification Authority of WoSign G2##FBEDDC9065B7272037BC550C9C56DEBBF27894E1##2014 Nov 08##2044 Nov 08##2048##SHA-256",
"Internet Security Research Group (ISRG)##Internet Security Research Group##ISRG Root X1##Approved, Pending Inclusion##CABD2A79A1076A31F21D253635CB039D4329A5E8##2015 Jun 04##2035 Jun 04##4096##SHA-256");

function is_known_rootCA( fingerprint ) {

  local_var fingerprint, ka, v;

  if( ! fingerprint ) return;

  foreach ka( known_CAs ) {

    v = split( ka, sep:"##", keep:FALSE );

    if( ! v ) return;

    if( tolower( v[ sha1_fingerprint ] ) == tolower( fingerprint ) )
      return TRUE;
  }

  return;
}
