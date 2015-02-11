/*
 * clone2fs.c - copy an entire ext2/ext3 file system.
 * Copyright (C) 2008 - 2010 Michael Riepe
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA
 */

static const char rcsid[] = "@(#) $Id: clone2fs.c,v 1.5 2010/01/31 23:32:37 michael Exp $";

#include <stddef.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <stdio.h>
#include <errno.h>
#include <time.h>
#include <assert.h>

#include <unistd.h>
#include <fcntl.h>

#include <ext2fs/ext2fs.h>

static const char *outfile = NULL;
static int overwrite = 0;
static int save = 0;
static int restore = 0;
static int verbose = 1;
static int zero_fill = 0;
static int stop_on_read_error = 1;

static int
xread(int fd, char *buf, size_t len) {
	ssize_t n;

	while (len) {
		n = read(fd, buf, len);
		if (n == -1) {
			if (errno == EINTR || errno == EAGAIN) {
				continue;
			}
			perror("read");
			return 1;
		}
		if (n == 0) {
			fprintf(stderr, "Premature EOF\n");
			return 1;
		}
		buf += n;
		len -= n;
	}
	return 0;
}

static int
xwrite(int fd, const char *buf, size_t len) {
	ssize_t n;

	while (len) {
		n = write(fd, buf, len);
		if (n == -1) {
			if (errno == EINTR || errno == EAGAIN) {
				continue;
			}
			perror("write");
			return 1;
		}
		if (n == 0) {
			fprintf(stderr, "Short write - out of disk space?\n");
			return 1;
		}
		buf += n;
		len -= n;
	}
	return 0;
}

static int
skip_bad_blocks(ext2_filsys fs) {
	ext2_badblocks_list bblist = NULL;
	ext2_badblocks_iterate iter;
	errcode_t res;
	blk_t blk;

	res = ext2fs_read_bb_inode(fs, &bblist);
	if (res) {
		com_err("ext2fs_read_bb_inode", res, "%s", fs->device_name);
		return -1;
	}
	res = ext2fs_badblocks_list_iterate_begin(bblist, &iter);
	if (res) {
		com_err("ext2fs_badblocks_list_iterate_begin", res, "%s", fs->device_name);
		ext2fs_badblocks_list_free(bblist);
		return -1;
	}
	while (ext2fs_badblocks_list_iterate(iter, &blk)) {
		if (verbose) {
			fprintf(stderr, "Skipping bad block %lu\n", (unsigned long)blk);
		}
		ext2fs_unmark_block_bitmap(fs->block_map, blk);
	}
	ext2fs_badblocks_list_iterate_end(iter);
	ext2fs_badblocks_list_free(bblist);
	return 0;
}

static void
show_progress(unsigned permille, unsigned run) {
	fprintf(stderr, "%u.%1u%% processed; %u:%02u:%02u elapsed\r",
		permille / 10, permille % 10, run / 3600, (run / 60) % 60, run % 60);
}

struct header {
	char magic[8];
	uint32_t version;
	uint32_t hdrsize;
	uint32_t blocksize;
	uint32_t blocks;
	uint32_t reserved[2];
};

#define CLONE2FS_IMG_MAGIC		"clone2fs"
#define CLONE2FS_IMG_VERSION	0x10000
#define CLONE2FS_IMG_HDRSIZE	sizeof(struct header)

static size_t
img_header_blocks(const struct header *p) {
	size_t bytes = p->hdrsize + (p->blocks + 7) / 8;
	return (bytes + p->blocksize - 1) / p->blocksize;
}

static int
write_image(ext2_filsys fs, int outfd) {
	char buffer[EXT2_MAX_BLOCK_SIZE];
	struct header hdr;
	errcode_t res;
	blk_t nblocks;
	blk_t nused;
	blk_t nblk;
	blk_t i;
	blk_t j;
	int can_seek;
	unsigned lastperm;
	unsigned permille;
	time_t start;

	res = ext2fs_read_block_bitmap(fs);
	if (res) {
		com_err("ext2fs_read_block_bitmap", res, "%s", fs->device_name);
		return -1;
	}
	if (skip_bad_blocks(fs)) {
		return -1;
	}
	nblocks = (blk_t)fs->super->s_blocks_count;
	if (verbose) {
		fprintf(stderr, "Cloning device: %s\n", fs->device_name);
		fprintf(stderr, "File system size: %lu blocks\n", (long)nblocks);
		fprintf(stderr, "Block size: %lu bytes\n", (long)fs->blocksize);
	}
	can_seek = lseek(outfd, (off_t)4096, SEEK_SET) == (off_t)4096;
	lseek(outfd, (off_t)0, SEEK_SET);
	nused = 0;
	if (save) {
		memset(&hdr, 0, sizeof(hdr));
		strncpy(hdr.magic, CLONE2FS_IMG_MAGIC, sizeof(hdr.magic));
		hdr.version = CLONE2FS_IMG_VERSION;
		hdr.hdrsize = CLONE2FS_IMG_HDRSIZE;
		hdr.blocksize = fs->blocksize;
		hdr.blocks = nblocks;
		memset(buffer, 0, fs->blocksize);
		memcpy(buffer, &hdr, sizeof(hdr));
		j = 8 * hdr.hdrsize;
		for (i = 0; i < hdr.blocks; ++i) {
			if (i == 0 || ext2fs_test_block_bitmap(fs->block_map, i)) {
				buffer[j / 8u] |= 1u << (j % 8u);
			}
			if (++j == 8u * fs->blocksize) {
				if (xwrite(outfd, buffer, fs->blocksize)) {
					return -1;
				}
				memset(buffer, 0, fs->blocksize);
				++nused;
				j = 0;
			}
		}
		if (j) {
			if (xwrite(outfd, buffer, fs->blocksize)) {
				return -1;
			}
			++nused;
		}
		assert(nused == img_header_blocks(&hdr));
		if (verbose) {
			fprintf(stderr, "Header blocks: %lu\n", (long)nused);
		}
	}
	lastperm = permille = 0;
	time(&start);
	for (nblk = i = 0; i < nblocks; ++i) {
		if (i == 0 || ext2fs_test_block_bitmap(fs->block_map, i)) {
			res = io_channel_read_blk(fs->io, i, 1, buffer);
			if (res) {
				com_err("io_channel_read_blk", res, "%s block %lu sector %llu",
					fs->device_name, (long)i, (long long)i * (fs->blocksize >> 9));
				if (stop_on_read_error) {
					return -1;
				}
				memset(buffer, 0, (size_t)fs->blocksize);
			}
			if (nblk != i) {
				assert(!save);
				assert(can_seek);
				assert(nblk < i);
				if (lseek(outfd, (off_t)i * fs->blocksize, SEEK_SET) == -1) {
					perror("lseek");
					return -1;
				}
			}
			if (xwrite(outfd, buffer, (size_t)fs->blocksize)) {
				return -1;
			}
			nblk = i + 1;
			++nused;
		}
		else if (save) {
			nblk = i + 1;	/* avoid seeking */
		}
		else if (zero_fill || !can_seek) {
			memset(buffer, 0, (size_t)fs->blocksize);
			if (xwrite(outfd, buffer, (size_t)fs->blocksize)) {
				return -1;
			}
			nblk = i + 1;
		}
		if (verbose) {
			permille = (i + 1) * 1000ull / nblocks;
			if (lastperm < permille) {
				lastperm = permille;
				show_progress(permille, (unsigned)difftime(time(NULL), start));
			}
		}
	}
	if (!save) {
		(void)ftruncate(outfd, (off_t)nblocks * fs->blocksize);
	}
	if (verbose) {
		fprintf(stderr, "\n%lu blocks written (%4.1f%%)\n",
			(unsigned long)nused, 1e2 * (double)nused / (double)nblocks);
	}
	return 0;
}

static int
restore_image(int infd, int outfd) {
	char buffer[EXT2_MAX_BLOCK_SIZE];
	char *bitmap = NULL;
	struct header hdr;
	blk_t nused;
	blk_t nblk;
	blk_t i;
	blk_t j;
	int can_seek;
	unsigned lastperm;
	unsigned permille;
	time_t start;
	size_t data_offset;

	if (xread(infd, buffer, 1024)) {
		return -1;
	}
	memcpy(&hdr, buffer, sizeof(struct header));
	if (strncmp(hdr.magic, CLONE2FS_IMG_MAGIC, sizeof(hdr.magic))) {
		fprintf(stderr, "Not a clone2fs image\n");
		return -1;
	}
	if (hdr.version != CLONE2FS_IMG_VERSION
	 || hdr.hdrsize != sizeof(struct header)) {
		fprintf(stderr, "Unknown header format\n");
		return -1;
	}
	data_offset = img_header_blocks(&hdr);
	if (verbose) {
		fprintf(stderr, "Image version: %u.%u.%u\n",
			(hdr.version >> 16), (hdr.version >> 8) & 0xff, hdr.version & 0xff);
		fprintf(stderr, "File system size: %lu blocks\n", (long)hdr.blocks);
		fprintf(stderr, "Block size: %lu bytes\n", (long)hdr.blocksize);
		fprintf(stderr, "Header blocks: %lu\n", (long)data_offset);
	}
	data_offset *= hdr.blocksize;
	bitmap = malloc(data_offset);
	if (bitmap == NULL) {
		perror("malloc");
		return -1;
	}
	memcpy(bitmap, buffer, 1024);
	if (data_offset > 1024) {
		if (xread(infd, bitmap + 1024, data_offset - 1024)) {
			free(bitmap);
			return -1;
		}
	}
	can_seek = lseek(outfd, (off_t)4096, SEEK_SET) == (off_t)4096;
	lseek(outfd, (off_t)0, SEEK_SET);
	nused = 0;
	lastperm = permille = 0;
	time(&start);
	j = 8 * hdr.hdrsize;
	for (nblk = i = 0; i < hdr.blocks; ++i, ++j) {
		if (bitmap[j / 8u] & (1u << (j % 8u))) {
			if (xread(infd, buffer, (size_t)hdr.blocksize)) {
				free(bitmap);
				return -1;
			}
			if (nblk != i) {
				assert(nblk < i);
				assert(can_seek);
				if (lseek(outfd, (off_t)i * hdr.blocksize, SEEK_SET) == -1) {
					perror("lseek");
					free(bitmap);
					return -1;
				}
			}
			if (xwrite(outfd, buffer, (size_t)hdr.blocksize)) {
				free(bitmap);
				return -1;
			}
			nblk = i + 1;
			++nused;
		}
		else if (zero_fill || !can_seek) {
			memset(buffer, 0, (size_t)hdr.blocksize);
			if (xwrite(outfd, buffer, (size_t)hdr.blocksize)) {
				free(bitmap);
				return -1;
			}
			nblk = i + 1;
		}
		if (verbose) {
			permille = (i + 1) * 1000ull / hdr.blocks;
			if (lastperm < permille) {
				lastperm = permille;
				show_progress(permille, (unsigned)difftime(time(NULL), start));
			}
		}
	}
	(void)ftruncate(outfd, (off_t)hdr.blocks * hdr.blocksize);
	if (verbose) {
		fprintf(stderr, "\n%lu blocks written (%4.1f%%)\n",
			(unsigned long)nused, 1e2 * (double)nused / (double)hdr.blocks);
	}
	free(bitmap);
	return 0;
}

static int
clone_ext2fs(const char *devname, int outfd) {
	int status = EXIT_FAILURE;

	if (restore) {
		int infd;

		if (strcmp(devname, "-") == 0) {
			devname = "standard output";
			infd = dup(STDIN_FILENO);
		}
		else {
			infd = open(devname, O_RDONLY, 0600);
		}
		if (infd == -1) {
			perror(devname);
		}
		else {
			if (!restore_image(infd, outfd)) {
				status = EXIT_SUCCESS;
			}
			close(infd);
		}
	}
	else {
		ext2_filsys fs;
		errcode_t res;
		int flags;

		if (ext2fs_check_if_mounted(devname, &flags) == 0
		 && (flags & (EXT2_MF_MOUNTED | EXT2_MF_READONLY)) == EXT2_MF_MOUNTED) {
			fprintf(stderr, "*** Warning: %s is currently in use.\n", devname);
			fprintf(stderr, "*** The resulting image may be inconsistent.\n");
			fprintf(stderr, "*** Run `e2fsck -f <image>' after %s.\n",
				save ? "restoring" : "copying");
		}
		res = ext2fs_open(devname, EXT2_FLAG_FORCE, 0, 0, unix_io_manager, &fs);
		if (res) {
			com_err("ext2fs_open", res, "%s", devname);
		}
		else {
			if (!write_image(fs, outfd)) {
				status = EXIT_SUCCESS;
			}
			ext2fs_free(fs);
		}
	}
	return status;
}

static void
usage(int status) {
	fprintf(stderr,
		"Usage: clone2fs [option...] (device|image)\n"
		"Options:\n"
		"  -o output   write image to <output>; \"-\" means stdout\n"
		"  -O output   write image to <output> even if that already exists\n"
		"  -Z          zero-fill unused blocks\n"
		"  -s          save to a compact (non-sparse) image\n"
		"  -r          restore from an image created with \"-s\"\n"
		"  -I          ignore read errors\n"
		"  -q          be less verbose\n"
		"  -h          display this help\n"
		"  -V          show program version and exit\n"
		"\n");
	exit(status);
}

static void
parse_options(int argc, char **argv) {
	int show_version = 0;
	int c;

	while ((c = getopt(argc, argv, "hIo:O:qrsVZ")) != -1) {
		switch (c) {
			default: break;
			case '?': usage(EXIT_FAILURE); break;
			case 'h': usage(EXIT_SUCCESS); break;
			case 'I': stop_on_read_error = 0; break;
			case 'O': overwrite = 1; outfile = optarg; break;
			case 'o': overwrite = 0; outfile = optarg; break;
			case 'q': verbose = 0; break;
			case 'r': restore = 1; break;
			case 's': save = 1; break;
			case 'V': show_version = 1; break;
			case 'Z': zero_fill = 1; break;
		}
	}
	if (save && zero_fill) {
		fprintf(stderr, "Ignoring meaningless -Z option in save mode.\n");
	}
	if (show_version) {
		const char *ver, *date;

		fprintf(stderr,
			"clone2fs " VERSION_STRING "\n"
			"Copyright (C) 2008 - 2010 Michael Riepe\n"
			"\n"
			"This program is free software; you can redistribute it and/or modify\n"
			"it under the terms of the GNU General Public License as published by\n"
			"the Free Software Foundation; either version 2 of the License, or\n"
			"(at your option) any later version.\n"
			"\n");
		ext2fs_get_library_version(&ver, &date);
		fprintf(stderr, "\tUsing EXT2FS Library version %s, %s\n\n", ver, date);
		exit(EXIT_SUCCESS);
	}
}

int
main(int argc, char **argv) {
	const char *dev;
	int flags;
	int outfd;
	int res;

	initialize_ext2_error_table();
	parse_options(argc, argv);
	if (optind >= argc) {
		fprintf(stderr, "Missing device argument.\n");
		usage(EXIT_FAILURE);
	}
	if (optind + 1 < argc) {
		fprintf(stderr, "Too many arguments.\n");
		usage(EXIT_FAILURE);
	}
	dev = argv[optind];
	if (outfile == NULL) {
		fprintf(stderr, "You must specify an output file.\n");
		usage(EXIT_FAILURE);
	}
	if (strcmp(outfile, "-") == 0) {
		outfile = "standard output";
		outfd = dup(STDOUT_FILENO);
	}
	else {
		if (ext2fs_check_if_mounted(outfile, &flags) == 0
		 && (flags & (EXT2_MF_MOUNTED | EXT2_MF_BUSY))) {
			fprintf(stderr, "%s is mounted or busy. Aborting.\n", outfile);
			exit(EXIT_FAILURE);
		}
		flags = O_RDWR | O_CREAT | (overwrite ? O_TRUNC : O_EXCL);
		outfd = open(outfile, flags, 0600);
	}
	if (outfd == -1) {
		perror(outfile);
		exit(EXIT_FAILURE);
	}
	if (isatty(outfd)) {
		fprintf(stderr, "Refusing to write to a terminal.\n");
		exit(EXIT_FAILURE);
	}
	res = clone_ext2fs(dev, outfd);
#if HAVE_FSYNC
	if (res == EXIT_SUCCESS) {
		if (verbose) {
			fprintf(stderr, "Syncing...\n");
		}
		fsync(outfd);
	}
#endif
	close(outfd);
	exit(res);
}
