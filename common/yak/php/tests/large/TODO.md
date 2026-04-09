# PHP Large Fixtures

- `large/` fixtures are deferred on purpose and should stay out of the default per-PR CI path.
- Default `syntax/` fixtures are expected to stay within the normal AST/build budget.
- Large fixtures remain opt-in TODO coverage until parser/runtime work brings them back under the default line.

Top-level large fixtures:

- `legacy_pinyin_payload.php`
- `legacy_qrcode_payload.php`

Project fixtures:

`bolt`
- `src__Storage__Mapping__MetadataDriver.php`

`cms`
- `src__Assets__Asset.php`
- `src__Auth__UserTags.php`
- `src__Dictionaries__Countries.php`
- `src__Entries__Collection.php`
- `src__Entries__Entry.php`
- `src__Fields__Blueprint.php`
- `src__Fieldtypes__Bard.php`
- `src__Http__Controllers__CP__Collections__CollectionsController.php`
- `src__Modifiers__CoreModifiers.php`
- `src__Providers__AddonServiceProvider.php`
- `src__View__Antlers__Language__Runtime__NodeProcessor.php`
- `tests__Antlers__Runtime__TemplateTest.php`
- `tests__Assets__AssetContainerTest.php`
- `tests__Assets__AssetTest.php`
- `tests__CP__Navigation__NavPreferencesNormalizerTest.php`
- `tests__CP__Navigation__NavPreferencesTest.php`
- `tests__Data__Entries__CollectionTest.php`
- `tests__Data__Entries__EntryTest.php`
- `tests__Data__Taxonomies__TermQueryBuilderTest.php`
- `tests__Fields__BlueprintTest.php`
- `tests__FrontendTest.php`
- `tests__Listeners__UpdateAssetReferencesTest.php`
- `tests__StarterKits__InitTest.php`
- `tests__StarterKits__InstallTest.php`
- `tests__Support__Concerns__TestsIlluminateArr.php`
- `tests__Support__Concerns__TestsIlluminateStr.php`
- `tests__Tags__Collection__EntriesTest.php`

`filament`
- `docs-assets__app__app__Livewire__TablesDemo.php`
- `packages__actions__src__Action.php`
- `packages__actions__src__ActionGroup.php`
- `packages__actions__src__Concerns__CanExportRecords.php`
- `packages__actions__src__Concerns__InteractsWithActions.php`
- `packages__actions__src__ImportAction.php`
- `packages__forms__src__Components__Builder.php`
- `packages__forms__src__Components__Concerns__CanBeValidated.php`
- `packages__forms__src__Components__ModalTableSelect.php`
- `packages__forms__src__Components__Select.php`
- `packages__infolists__src__Components__TextEntry.php`
- `packages__panels__src__Commands__MakePageCommand.php`
- `packages__panels__src__Commands__MakeRelationManagerCommand.php`
- `packages__tables__src__Columns__SelectColumn.php`
- `tests__src__Forms__Components__SelectTest.php`
- `tests__src__Panels__Commands__MakeResourceCommandTest.php`
- `tests__src__Tables__ColumnTest.php`
- `tests__src__Tables__Filters__QueryBuilderTest.php`
- `tests__src__Tables__Filters__SelectFilterTest.php`

`grav`
- `system__src__Grav__Common__Debugger.php`
- `system__src__Grav__Common__Grav.php`
- `system__src__Grav__Common__Page__Page.php`
- `system__src__Grav__Common__Page__Pages.php`
- `system__src__Grav__Common__Twig__Extension__GravExtension.php`
- `system__src__Grav__Common__Utils.php`
- `system__src__Grav__Console__Gpm__InstallCommand.php`
- `system__src__Grav__Framework__Flex__FlexIndex.php`
- `system__src__Grav__Framework__Flex__FlexObject.php`

`pfsense`
- `diag_packet_capture.php`
- `filter.inc`
- `firewall_rules_edit.php`
- `guiconfig.inc`
- `interfaces.inc`
- `ipsec.inc`
- `pkg_edit.php`
- `rrd.inc`
- `service-utils.inc`
- `services.inc`
- `services_captiveportal.php`
- `shaper.inc`
- `status_dhcpv6_leases.php`
- `status_logs_settings.php`
- `syslog.inc`
- `system.inc`
- `system_information.widget.php`
- `upgrade_config.inc`
- `util.inc`
- `wizard.php`

`prestashop`
- `classes__Tools.php`
- `classes__controller__AdminController.php`
- `classes__module__Module.php`
- `src__Adapter__MailTemplate__MailPreviewVariablesBuilder.php`
- `src__PrestaShopBundle__Install__Install.php`
- `tests__Integration__Behaviour__Features__Context__CommonFeatureContext.php`
- `tests__Integration__Behaviour__Features__Context__Domain__Carrier__CarrierFeatureContext.php`
- `tests__Integration__Behaviour__Features__Context__Domain__CartFeatureContext.php`
- `tests__Integration__Behaviour__Features__Context__Domain__Discount__DiscountFeatureContext.php`
- `tests__Integration__Behaviour__Features__Context__Domain__OrderFeatureContext.php`

`qloapps`
- `classes__controller__AdminController.php`
- `controllers__admin__AdminImportController.php`
- `controllers__admin__AdminNormalProductsController.php`
- `controllers__admin__AdminOrdersController.php`
- `controllers__admin__AdminProductsController.php`
- `tools__smarty__sysplugins__smarty_internal_templateparser.php`
- `tools__tcpdf__tcpdf.php`

`twill`
- `src__Http__Controllers__Admin__ModuleController.php`
