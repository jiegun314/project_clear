// Native file/folder choosers that marshal onto the macOS main queue.
//
// Wails v2 dispatches bound methods on a goroutine and then presents
// NSOpenPanel straight from that thread. AppKit is not thread safe, and the
// result is a panel that appears and is torn down again immediately. Every
// call here hops to the main queue first and runs the panel modally, which is
// what AppKit expects and what keeps it on screen until the user is done.

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>

extern void clearPanelResult(char *json);

// JSON-encode one string, quotes included.
//
// Do not reach for NSJSONSerialization here: the only call that encodes a
// bare string is dataWithJSONObject:@[ s ], which yields ["s"] -- an array
// nested inside the paths array. Go then fails to decode it with "cannot
// unmarshal array into ... of type string" and the whole result is dropped, so
// picking files and pressing OK silently yielded nothing. (Encoding a bare
// top-level string needs NSJSONWritingFragmentsAllowed, which is 10.15+;
// LSMinimumSystemVersion here is 10.13, so this is written out by hand.)
static NSString *_clear_json_string(NSString *s) {
    if (s == nil) {
        return @"\"\"";
    }
    NSMutableString *o = [NSMutableString stringWithString:@"\""];
    NSUInteger n = [s length];
    for (NSUInteger i = 0; i < n; i++) {
        unichar c = [s characterAtIndex:i];
        switch (c) {
            case '"':  [o appendString:@"\\\""]; break;
            case '\\': [o appendString:@"\\\\"]; break;
            case '\b': [o appendString:@"\\b"];  break;
            case '\f': [o appendString:@"\\f"];  break;
            case '\n': [o appendString:@"\\n"];  break;
            case '\r': [o appendString:@"\\r"];  break;
            case '\t': [o appendString:@"\\t"];  break;
            default:
                if (c < 0x20) {
                    [o appendFormat:@"\\u%04x", (unsigned)c];
                } else {
                    [o appendFormat:@"%C", c];
                }
                break;
        }
    }
    [o appendString:@"\""];
    return o;
}

static void sendResult(BOOL canceled, NSArray<NSString *> *paths, NSString *err) {
    NSMutableString *out = [NSMutableString stringWithString:@"{"];
    [out appendString:@"\"canceled\":"];
    [out appendString:canceled ? @"true" : @"false"];
    if (!canceled) {
        [out appendString:@",\"paths\":["];
        for (NSUInteger i = 0; i < paths.count; i++) {
            if (i > 0) {
                [out appendString:@","];
            }
            [out appendString:_clear_json_string(paths[i])];
        }
        [out appendString:@"]"];
    }
    if (err != nil) {
        [out appendFormat:@",\"error\":%@", _clear_json_string(err)];
    }
    [out appendString:@"}"];

    // Hand the payload over as a heap copy. [out UTF8String] points into
    // storage owned by `out` and was never malloc'd, while clearPanelResult
    // frees whatever it receives. Freeing that pointer trips the macOS malloc
    // guard and kills the process with SIGTRAP; because a Go fatal error goes
    // to stderr and exits 2, and stderr is invisible when the app is launched
    // from Finder, it looks like the app simply vanished. strdup makes the
    // ownership contract explicit: the callee always owns a malloc'd buffer.
    char *payload = strdup([out UTF8String]);
    if (payload == NULL) {
        return;
    }
    clearPanelResult(payload);
}

// NSWindow the sheet should belong to: the key window, falling back to the
// first visible window of the app.
static NSWindow *_clear_sheet_parent(void) {
    NSApplication *app = [NSApplication sharedApplication];
    NSWindow *key = [app keyWindow];
    if (key != nil && [key isVisible]) {
        return key;
    }
    for (NSWindow *w in [app windows]) {
        if ([w isVisible]) {
            return w;
        }
    }
    return nil;
}

static NSArray<NSString *> *_clear_extensions(const char *filters) {
    NSMutableArray *out = [NSMutableArray array];
    if (filters == NULL) {
        return out;
    }
    NSString *raw = [NSString stringWithUTF8String:filters];
    for (NSString *part in [raw componentsSeparatedByString:@";"]) {
        NSString *p = [part stringByTrimmingCharactersInSet:
                       [NSCharacterSet whitespaceCharacterSet]];
        p = [p stringByReplacingOccurrencesOfString:@"*." withString:@""];
        p = [p stringByReplacingOccurrencesOfString:@" " withString:@""];
        if ([p length] > 0) {
            [out addObject:[p lowercaseString]];
        }
    }
    return out;
}

void clearOpenPanel(const char *title, const char *dir, int allowFiles, int allowDirs,
                    int multiple, const char *filters, const char *buttonLabel) {
    // Hop to the main queue: this is the whole point of this file.
    dispatch_async(dispatch_get_main_queue(), ^{
        NSOpenPanel *panel = [NSOpenPanel openPanel];
        if (title != NULL && strlen(title) > 0) {
            [panel setTitle:[NSString stringWithUTF8String:title]];
        }
        [panel setCanChooseFiles:allowFiles ? YES : NO];
        [panel setCanChooseDirectories:allowDirs ? YES : NO];
        [panel setAllowsMultipleSelection:multiple ? YES : NO];
        [panel setCanCreateDirectories:YES];
        [panel setResolvesAliases:YES];

        if (dir != NULL && strlen(dir) > 0) {
            NSString *d = [NSString stringWithUTF8String:dir];
            if ([[NSFileManager defaultManager] fileExistsAtPath:d]) {
                [panel setDirectoryURL:[NSURL fileURLWithPath:d isDirectory:YES]];
            }
        }

        NSArray *exts = _clear_extensions(filters);
        if ([exts count] > 0) {
            if (@available(macOS 12.0, *)) {
                NSMutableArray<UTType *> *types = [NSMutableArray array];
                for (NSString *e in exts) {
                    UTType *t = [UTType typeWithFilenameExtension:e];
                    if (t != nil) {
                        [types addObject:t];
                    }
                }
                [panel setAllowedContentTypes:types];
                [panel setAllowsOtherFileTypes:NO];
            } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
                [panel setAllowedFileTypes:exts];
                [panel setAllowsOtherFileTypes:NO];
#pragma clang diagnostic pop
            }
        }
        if (buttonLabel != NULL && strlen(buttonLabel) > 0) {
            [panel setPrompt:[NSString stringWithUTF8String:buttonLabel]];
        }

        // Bring the app forward first so the panel is what the user sees, then
        // run it modally. A sheet is discarded outright when its parent window
        // is not the key window of the frontmost app, which is exactly the
        // "panel flashes and disappears" failure this file exists to avoid.
        [NSApp activateIgnoringOtherApps:YES];
        NSWindow *parent = _clear_sheet_parent();
        [parent makeKeyAndOrderFront:nil];

        NSModalResponse rc = [panel runModal];
        if (rc != NSModalResponseOK) {
            sendResult(YES, @[], nil);
            return;
        }
        NSMutableArray<NSString *> *paths = [NSMutableArray array];
        for (NSURL *u in [panel URLs]) {
            [paths addObject:[u path]];
        }
        sendResult(NO, paths, nil);
    });
}

void clearSavePanel(const char *title, const char *filename, const char *dir,
                    const char *filters, const char *buttonLabel) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSSavePanel *panel = [NSSavePanel savePanel];
        [panel setExtensionHidden:NO];
        [panel setCanCreateDirectories:YES];
        if (title != NULL && strlen(title) > 0) {
            [panel setTitle:[NSString stringWithUTF8String:title]];
        }
        if (filename != NULL && strlen(filename) > 0) {
            [panel setNameFieldStringValue:[NSString stringWithUTF8String:filename]];
        }
        if (dir != NULL && strlen(dir) > 0) {
            NSString *d = [NSString stringWithUTF8String:dir];
            if ([[NSFileManager defaultManager] fileExistsAtPath:d]) {
                [panel setDirectoryURL:[NSURL fileURLWithPath:d isDirectory:YES]];
            }
        }
        NSArray *exts = _clear_extensions(filters);
        if ([exts count] > 0) {
            if (@available(macOS 12.0, *)) {
                NSMutableArray<UTType *> *types = [NSMutableArray array];
                for (NSString *e in exts) {
                    UTType *t = [UTType typeWithFilenameExtension:e];
                    if (t != nil) {
                        [types addObject:t];
                    }
                }
                [panel setAllowedContentTypes:types];
            } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
                [panel setAllowedFileTypes:exts];
#pragma clang diagnostic pop
            }
        }
        if (buttonLabel != NULL && strlen(buttonLabel) > 0) {
            [panel setPrompt:[NSString stringWithUTF8String:buttonLabel]];
        }

        [NSApp activateIgnoringOtherApps:YES];
        NSWindow *sparent = _clear_sheet_parent();
        [sparent makeKeyAndOrderFront:nil];
        NSModalResponse rc = [panel runModal];
        if (rc != NSModalResponseOK) {
            sendResult(YES, @[], nil);
            return;
        }
        NSString *path = [[panel URL] path];
        sendResult(NO, path != nil ? @[ path ] : @[], nil);
    });
}

// clearTestSendResult drives sendResult without presenting a panel, so the Go
// test can exercise the real hand-off. It exists because the failure it guards
// against takes the process down outright: a Go fatal error prints to stderr
// and exits 2, which reads as the app silently vanishing when it is launched
// from Finder, so the app's own logging never records it.
void clearTestSendResult(int canceled, int pathCount) {
    if (canceled) {
        sendResult(YES, @[], nil);
        return;
    }
    NSMutableArray<NSString *> *paths = [NSMutableArray array];
    for (int i = 0; i < pathCount; i++) {
        // Deliberately non-ASCII: exercises the UTF-8 hand-off as well.
        [paths addObject:[NSString stringWithFormat:@"/tmp/测试 路径 %d.xlsm", i]];
    }
    sendResult(NO, paths, nil);
}

// clearTestSendTrickyPaths sends paths that need JSON escaping, which real
// paths do contain: quotes, backslashes and non-ASCII names are all legal on
// macOS.
void clearTestSendTrickyPaths(void) {
    sendResult(NO, @[
        @"/tmp/a\"b.xlsm",
        @"/tmp/back\\slash.xlsm",
        @"/tmp/tab\there.xlsm",
        @"/tmp/换行\nhere.xlsm",
        @"/tmp/plain.xlsm",
    ], nil);
}
