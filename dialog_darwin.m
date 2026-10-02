// Native file/folder choosers that marshal onto the macOS main queue.
//
// Wails v2 dispatches bound methods on a goroutine and then presents
// NSOpenPanel straight from that thread. AppKit is not thread safe, and the
// result is a panel that appears and is torn down again immediately. Every
// call here hops to the main queue first, so the sheet is presented the way
// AppKit expects and stays on screen until the user is done with it.

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>

extern void clearPanelResult(char *json);

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
            NSData *d = [NSJSONSerialization dataWithJSONObject:@[ paths[i] ]
                                                      options:0
                                                        error:nil];
            NSString *s = [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
            [out appendString:s];
        }
        [out appendString:@"]"];
    }
    if (err != nil) {
        NSData *d = [NSJSONSerialization dataWithJSONObject:@[ err ] options:0 error:nil];
        NSString *s = [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
        [out appendFormat:@",\"error\":%@", s];
    }
    [out appendString:@"}"];
    clearPanelResult((char *)[out UTF8String]);
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
