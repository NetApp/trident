# Change Log

[Releases](https://github.com/NetApp/trident/releases)

## Changes since 1.0

**Fixes:**

- Trident now rejects ONTAP backends with no aggregates assigned to the SVM.
- Trident now allows ONTAP backends even if it cannot read the aggregate media type,
or if the media type is unknown. However, such backends will be ignored for storage
classes that require a specific media type.

**Enhancements:**

- Added release notes (CHANGELOG.md)
- Enabled SVM-scoped users for ONTAP backends.
- Switched to using vserver-show-aggr-get-iter API for ONTAP 9.0 and later to get aggregate
media type.
