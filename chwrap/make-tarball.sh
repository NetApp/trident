#!/bin/sh -e

[ -n "$1" ] && [ -n "$2" ] || exit 1

PREFIX=$(mktemp -d)
mkdir -p $PREFIX/netapp
cp "$1" $PREFIX/netapp/chwrap
for BIN in apt blkid blockdev cat cryptsetup dd df dnf docker e2fsck free fsck.ext3 fsck.ext4 iscsiadm losetup ls lsblk lsscsi \
mkdir lsmod mkfs.ext3 mkfs.ext4 mkfs.xfs mount mount.nfs mount.nfs4 mpathconf multipath multipathd nvme pgrep resize2fs rmdir \
rpcinfo stat systemctl umount xfs_growfs yum ; do
  ln -s chwrap $PREFIX/netapp/$BIN
done

tar --owner=0 --group=0 -C $PREFIX -cf "$2" netapp ||\
  (echo "gnu tar failed, attempting darwin tar"; tar --uid=0 --gid=0 -C $PREFIX -cf "$2" netapp)

rm -rf $PREFIX
