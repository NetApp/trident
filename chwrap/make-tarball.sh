#!/bin/sh -e

[ -n "$1" ] && [ -n "$2" ] || exit 1

PREFIX=/tmp/$(uuidgen)
mkdir -p $PREFIX/netapp
cp "$1" $PREFIX/netapp/chwrap
for BIN in blkid cat dd df free iscsiadm ls lsblk lsscsi mkdir mkfs.ext3 mkfs.ext4 mkfs.xfs mount multipath multipathd pgrep resize2fs rmdir stat umount xfs_growfs ; do
  ln -s chwrap $PREFIX/netapp/$BIN
done
tar --owner=0 --group=0 -C $PREFIX -cf "$2" netapp
rm -rf $PREFIX
