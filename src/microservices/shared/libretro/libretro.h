/* Copyright (c) 2010-2024 The RetroArch team
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

#ifndef LIBRETRO_H__
#define LIBRETRO_H__

#include <stdint.h>
#include <stddef.h>
#include <limits.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifndef __cplusplus
#include <stdbool.h>
#endif

#define RETRO_API_VERSION 1

/* Video pixel formats */
#define RETRO_PIXEL_FORMAT_0RGB1555 0
#define RETRO_PIXEL_FORMAT_XRGB8888 1
#define RETRO_PIXEL_FORMAT_RGB565   2

/* Environment commands */
#define RETRO_ENVIRONMENT_SET_ROTATION          1
#define RETRO_ENVIRONMENT_GET_OVERSCAN          2
#define RETRO_ENVIRONMENT_GET_CAN_DUPE          3
#define RETRO_ENVIRONMENT_SET_MESSAGE           6
#define RETRO_ENVIRONMENT_SHUTDOWN              7
#define RETRO_ENVIRONMENT_SET_PERFORMANCE_LEVEL 8
#define RETRO_ENVIRONMENT_GET_SYSTEM_DIRECTORY  9
#define RETRO_ENVIRONMENT_SET_PIXEL_FORMAT      10
#define RETRO_ENVIRONMENT_SET_INPUT_DESCRIPTORS 11
#define RETRO_ENVIRONMENT_SET_KEYBOARD_CALLBACK 12
#define RETRO_ENVIRONMENT_SET_DISK_CONTROL_INTERFACE 13
#define RETRO_ENVIRONMENT_SET_HW_RENDER         14
#define RETRO_ENVIRONMENT_GET_VARIABLE          15
#define RETRO_ENVIRONMENT_SET_VARIABLES         16
#define RETRO_ENVIRONMENT_GET_VARIABLE_UPDATE   17
#define RETRO_ENVIRONMENT_SET_SUPPORT_NO_GAME   18
#define RETRO_ENVIRONMENT_GET_LIBRETRO_PATH     19
#define RETRO_ENVIRONMENT_SET_FRAME_TIME_CALLBACK 21
#define RETRO_ENVIRONMENT_SET_AUDIO_CALLBACK    22
#define RETRO_ENVIRONMENT_GET_RUMBLE_INTERFACE  23
#define RETRO_ENVIRONMENT_GET_INPUT_DEVICE_CAPABILITIES 24
#define RETRO_ENVIRONMENT_GET_SENSOR_INTERFACE  (25 | 0x80000)
#define RETRO_ENVIRONMENT_GET_CAMERA_INTERFACE  (26 | 0x80000)
#define RETRO_ENVIRONMENT_GET_LOG_INTERFACE     27
#define RETRO_ENVIRONMENT_GET_PERF_INTERFACE    28
#define RETRO_ENVIRONMENT_GET_LOCATION_INTERFACE 29
#define RETRO_ENVIRONMENT_GET_CORE_ASSETS_DIRECTORY 30
#define RETRO_ENVIRONMENT_GET_SAVE_DIRECTORY    31
#define RETRO_ENVIRONMENT_SET_SYSTEM_AV_INFO    32
#define RETRO_ENVIRONMENT_SET_PROC_ADDRESS_CALLBACK 33
#define RETRO_ENVIRONMENT_SET_SUBSYSTEM_INFO    34
#define RETRO_ENVIRONMENT_SET_CONTROLLER_INFO   35
#define RETRO_ENVIRONMENT_SET_MEMORY_MAPS       (36 | 0x80000)
#define RETRO_ENVIRONMENT_SET_GEOMETRY          37
#define RETRO_ENVIRONMENT_GET_USERNAME          38
#define RETRO_ENVIRONMENT_GET_LANGUAGE          39
#define RETRO_ENVIRONMENT_GET_CURRENT_SOFTWARE_FRAMEBUFFER (40 | 0x80000)
#define RETRO_ENVIRONMENT_GET_HW_RENDER_INTERFACE (41 | 0x80000)
#define RETRO_ENVIRONMENT_SET_SUPPORT_ACHIEVEMENTS (42 | 0x80000)
#define RETRO_ENVIRONMENT_SET_HW_RENDER_CONTEXT_NEGOTIATION_INTERFACE (43 | 0x80000)
#define RETRO_ENVIRONMENT_SET_SERIALIZATION_QUIRKS 44
#define RETRO_ENVIRONMENT_SET_HW_SHARED_CONTEXT (44 | 0x80000)
#define RETRO_ENVIRONMENT_GET_VFS_INTERFACE     (45 | 0x80000)
#define RETRO_ENVIRONMENT_GET_LED_INTERFACE     (46 | 0x80000)
#define RETRO_ENVIRONMENT_GET_AUDIO_VIDEO_ENABLE (47 | 0x80000)
#define RETRO_ENVIRONMENT_GET_MIDI_INTERFACE    (48 | 0x80000)
#define RETRO_ENVIRONMENT_GET_FASTFORWARDING    (49 | 0x80000)
#define RETRO_ENVIRONMENT_GET_TARGET_REFRESH_RATE (50 | 0x80000)
#define RETRO_ENVIRONMENT_GET_INPUT_BITMASKS    (51 | 0x80000)
#define RETRO_ENVIRONMENT_GET_CORE_OPTIONS_VERSION 52
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS      53
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS_INTL 54
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS_DISPLAY 55
#define RETRO_ENVIRONMENT_GET_PREFERRED_HW_RENDER 56
#define RETRO_ENVIRONMENT_GET_DISK_CONTROL_INTERFACE_VERSION 57
#define RETRO_ENVIRONMENT_SET_DISK_CONTROL_EXTENDED_INTERFACE 58
#define RETRO_ENVIRONMENT_GET_MESSAGE_INTERFACE_VERSION 59
#define RETRO_ENVIRONMENT_SET_MESSAGE_EXT      60
#define RETRO_ENVIRONMENT_GET_INPUT_MAX_USERS  61
#define RETRO_ENVIRONMENT_SET_AUDIO_BUFFER_STATUS_CALLBACK 62
#define RETRO_ENVIRONMENT_SET_MINIMUM_AUDIO_LATENCY 63
#define RETRO_ENVIRONMENT_SET_FASTFORWARDING_OVERRIDE 64
#define RETRO_ENVIRONMENT_SET_CONTENT_INFO_OVERRIDE 65
#define RETRO_ENVIRONMENT_GET_GAME_INFO_OVERRIDE 66
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS_V2  67
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS_V2_INTL 68
#define RETRO_ENVIRONMENT_SET_CORE_OPTIONS_UPDATE_DISPLAY_CALLBACK 69
#define RETRO_ENVIRONMENT_SET_VARIABLE         70
#define RETRO_ENVIRONMENT_GET_THROTTLE_STATE   (71 | 0x80000)
#define RETRO_ENVIRONMENT_GET_SAVESTATE_CONTEXT (72 | 0x80000)
#define RETRO_ENVIRONMENT_GET_HW_RENDER_CONTEXT_NEGOTIATION_INTERFACE_SUPPORT (73 | 0x80000)
#define RETRO_ENVIRONMENT_GET_JIT_CAPABLE      74
#define RETRO_ENVIRONMENT_GET_MICROPHONE_INTERFACE (75 | 0x80000)
#define RETRO_ENVIRONMENT_SET_NETPACKET_INTERFACE 76
#define RETRO_ENVIRONMENT_GET_DEVICE_POWER     (77 | 0x80000)
#define RETRO_ENVIRONMENT_SET_SUPPORT_DEVICE_POWER_OFF (78 | 0x80000)

/* Device types for retro_input_state_t */
#define RETRO_DEVICE_TYPE_SHIFT 8
#define RETRO_DEVICE_MASK       ((1 << RETRO_DEVICE_TYPE_SHIFT) - 1)
#define RETRO_DEVICE_SUBCLASS(base, id) (((id + 1) << RETRO_DEVICE_TYPE_SHIFT) | base)

#define RETRO_DEVICE_NONE         0
#define RETRO_DEVICE_JOYPAD       1
#define RETRO_DEVICE_MOUSE        2
#define RETRO_DEVICE_KEYBOARD     3
#define RETRO_DEVICE_LIGHTGUN     4
#define RETRO_DEVICE_ANALOG       5
#define RETRO_DEVICE_POINTER      6

/* Joypad buttons */
#define RETRO_DEVICE_ID_JOYPAD_B        0
#define RETRO_DEVICE_ID_JOYPAD_Y        1
#define RETRO_DEVICE_ID_JOYPAD_SELECT   2
#define RETRO_DEVICE_ID_JOYPAD_START    3
#define RETRO_DEVICE_ID_JOYPAD_UP       4
#define RETRO_DEVICE_ID_JOYPAD_DOWN     5
#define RETRO_DEVICE_ID_JOYPAD_LEFT     6
#define RETRO_DEVICE_ID_JOYPAD_RIGHT    7
#define RETRO_DEVICE_ID_JOYPAD_A        8
#define RETRO_DEVICE_ID_JOYPAD_X        9
#define RETRO_DEVICE_ID_JOYPAD_L       10
#define RETRO_DEVICE_ID_JOYPAD_R       11
#define RETRO_DEVICE_ID_JOYPAD_L2      12
#define RETRO_DEVICE_ID_JOYPAD_R2      13
#define RETRO_DEVICE_ID_JOYPAD_L3      14
#define RETRO_DEVICE_ID_JOYPAD_R3      15

/* Memory types for RETRO_ENVIRONMENT_SET_MEMORY_MAPS */
#define RETRO_MEMORY_MASK                0xff
#define RETRO_MEMORY_SAVE_RAM            0
#define RETRO_MEMORY_RTC                 1
#define RETRO_MEMORY_SYSTEM_RAM          2
#define RETRO_MEMORY_VIDEO_RAM           3

/* Region */
#define RETRO_REGION_NTSC  0
#define RETRO_REGION_PAL   1

/* Language IDs */
#define RETRO_LANGUAGE_ENGLISH            0
#define RETRO_LANGUAGE_JAPANESE           1
#define RETRO_LANGUAGE_FRENCH             2

/* Key IDs for keyboard callback */
#define RETROK_UNKNOWN       0
#define RETROK_RETURN       13
#define RETROK_ESCAPE       27
#define RETROK_BACKSPACE    8
#define RETROK_TAB           9
#define RETROK_SPACE        32
#define RETROK_UP          1073741906
#define RETROK_DOWN        1073741905
#define RETROK_LEFT        1073741904
#define RETROK_RIGHT       1073741903
#define RETROK_a           97
#define RETROK_b           98
#define RETROK_c           99
#define RETROK_d           100
#define RETROK_e           101
#define RETROK_f           102
#define RETROK_g           103
#define RETROK_h           104
#define RETROK_i           105
#define RETROK_j           106
#define RETROK_k           107
#define RETROK_l           108
#define RETROK_m           109
#define RETROK_n           110
#define RETROK_o           111
#define RETROK_p           112
#define RETROK_q           113
#define RETROK_r           114
#define RETROK_s           115
#define RETROK_t           116
#define RETROK_u           117
#define RETROK_v           118
#define RETROK_w           119
#define RETROK_x           120
#define RETROK_y           121
#define RETROK_z           122

/* Key modifiers */
#define RETROKMOD_NONE      0x0000
#define RETROKMOD_SHIFT     0x01
#define RETROKMOD_CTRL      0x02
#define RETROKMOD_ALT       0x04
#define RETROKMOD_META      0x08
#define RETROKMOD_CAPSLOCK  0x10
#define RETROKMOD_NUMLOCK   0x20
#define RETROKMOD_SCROLLOCK 0x40

/* retro_system_info */
struct retro_system_info {
   const char *library_name;
   const char *library_version;
   const char *valid_extensions;
   bool        need_fullpath;
   bool        block_extract;
};

/* retro_game_geometry */
struct retro_game_geometry {
   unsigned base_width;
   unsigned base_height;
   unsigned max_width;
   unsigned max_height;
   float    aspect_ratio;
};

/* retro_system_timing */
struct retro_system_timing {
   double fps;
   double sample_rate;
};

/* retro_system_av_info */
struct retro_system_av_info {
   struct retro_game_geometry geometry;
   struct retro_system_timing timing;
};

/* retro_variable */
struct retro_variable {
   const char *key;
   const char *value;
};

/* retro_game_info */
struct retro_game_info {
   const char *path;
   const void *data;
   size_t      size;
   const char *meta;
};

/* retro_input_descriptor */
struct retro_input_descriptor {
   unsigned port;
   unsigned device;
   unsigned index;
   unsigned id;
   const char *description;
};

/* retro_memory_descriptor */
struct retro_memory_descriptor {
   uint64_t flags;
   void     *ptr;
   size_t    offset;
   size_t    start;
   size_t    select;
   size_t    disconnect;
   size_t    len;
   const char *addrspace;
};

/* retro_memory_map */
struct retro_memory_map {
   struct retro_memory_descriptor *descriptors;
   unsigned num_descriptors;
};

/* retro_controller_description */
struct retro_controller_description {
   const char *desc;
   unsigned    id;
};

/* retro_controller_info */
struct retro_controller_info {
   struct retro_controller_description *types;
   unsigned num_types;
};

/* retro_subsystem_memory_info */
struct retro_subsystem_memory_info {
   const char *extension;
   unsigned    type;
};

/* retro_subsystem_rom_info */
struct retro_subsystem_rom_info {
   const char *desc;
   const char *valid_extensions;
   const char *required_memory;
   unsigned    num_memory;
   const struct retro_subsystem_memory_info *memory;
};

/* retro_subsystem_info */
struct retro_subsystem_info {
   const char *desc;
   const char *ident;
   const struct retro_subsystem_rom_info *roms;
   unsigned num_roms;
   unsigned id;
};

/* retro_fastforwarding_override */
struct retro_fastforwarding_override {
   double ratio;
   bool   fastforward;
   bool   notification;
   bool   inhibit_toggle;
};

/* retro_frame_time_callback */
struct retro_frame_time_callback {
   void (*callback)(int64_t usec);
   int64_t reference;
};

/* retro_rumble_interface (RETRO_ENVIRONMENT_GET_RUMBLE_INTERFACE) — this
 * block used to be a `struct retro_rumble_effect { int16_t strong_magnitude,
 * weak_magnitude; }`, which is NOT part of the real libretro API (no such
 * struct exists upstream) and had zero usages anywhere in this codebase —
 * removed rather than kept, since its tag name collides with the REAL
 * `enum retro_rumble_effect` C requires here (struct/enum/union tags share
 * one namespace) and it served no purpose. The actual rumble API surface
 * was simply never added to this stripped-down header before now — added
 * here verbatim from the official libretro.h, same convention the
 * hw_render_callback block below already follows for its own addition.
 */
enum retro_rumble_effect
{
   RETRO_RUMBLE_STRONG = 0,
   RETRO_RUMBLE_WEAK = 1
   /* RETRO_RUMBLE_DUMMY = INT_MAX in the real libretro.h — a padding value
    * only, forcing the enum to sizeof(int); omitted rather than pulling in
    * <limits.h> for a value nothing here ever reads. */
};

typedef bool (*retro_set_rumble_state_t)(unsigned port, enum retro_rumble_effect effect, uint16_t strength);

struct retro_rumble_interface
{
   retro_set_rumble_state_t set_rumble_state;
};

/* retro_disk_control_callback (RETRO_ENVIRONMENT_SET_DISK_CONTROL_INTERFACE)
 * — comment un jeu en plusieurs disques change de disque en cours de partie.
 * Un .m3u et ses .cue : le core lit la liste, expose ces sept pointeurs, et
 * c'est le frontend qui pilote. La séquence est toujours la même — éjecter,
 * choisir l'image, refermer — et rien d'autre ne la déclenche.
 *
 * Verbatim du libretro.h officiel (cette copie vendorisée est par ailleurs un
 * sous-ensemble). replace_image_index/add_image_index sont là pour que la
 * struct ait la BONNE TAILLE et le bon ordre de champs : le core écrit sept
 * pointeurs, en lire cinq dans une struct plus courte lirait à côté. On ne
 * les appelle pas.
 */
typedef bool (*retro_set_eject_state_t)(bool ejected);
typedef bool (*retro_get_eject_state_t)(void);
typedef unsigned (*retro_get_image_index_t)(void);
typedef bool (*retro_set_image_index_t)(unsigned index);
typedef unsigned (*retro_get_num_images_t)(void);
typedef bool (*retro_replace_image_index_t)(unsigned index, const struct retro_game_info *info);
typedef bool (*retro_add_image_index_t)(void);

struct retro_disk_control_callback
{
   retro_set_eject_state_t     set_eject_state;
   retro_get_eject_state_t     get_eject_state;

   retro_get_image_index_t     get_image_index;
   retro_set_image_index_t     set_image_index;
   retro_get_num_images_t      get_num_images;

   retro_replace_image_index_t replace_image_index;
   retro_add_image_index_t     add_image_index;
};

/* retro_audio_callback */
struct retro_audio_callback {
   void (*callback)(void);
   unsigned frames;
};

/* retro_hw_render_callback (RETRO_ENVIRONMENT_SET_HW_RENDER) — added for
 * hardware-rendered cores (N64/parallel_n64, PS2/play), which have no
 * software-rendering path unlike every other core this backend runs.
 * Verbatim from the official libretro.h (this vendored copy is otherwise a
 * stripped-down subset) — RETRO_CALLCONV is empty on this platform
 * (x86_64 Linux, cdecl is the only convention that matters), so it's
 * omitted entirely rather than defined as a no-op macro.
 */
enum retro_hw_context_type {
   RETRO_HW_CONTEXT_NONE             = 0,
   RETRO_HW_CONTEXT_OPENGL           = 1,
   RETRO_HW_CONTEXT_OPENGLES2        = 2,
   RETRO_HW_CONTEXT_OPENGL_CORE      = 3,
   RETRO_HW_CONTEXT_OPENGLES3        = 4,
   RETRO_HW_CONTEXT_OPENGLES_VERSION = 5,
   RETRO_HW_CONTEXT_VULKAN           = 6,
   RETRO_HW_CONTEXT_D3D11            = 7,
   RETRO_HW_CONTEXT_D3D10            = 8,
   RETRO_HW_CONTEXT_D3D12            = 9,
   RETRO_HW_CONTEXT_D3D9             = 10,
   RETRO_HW_CONTEXT_DUMMY            = INT_MAX
};

typedef void (*retro_proc_address_t)(void);
typedef uintptr_t (*retro_hw_get_current_framebuffer_t)(void);
typedef retro_proc_address_t (*retro_hw_get_proc_address_t)(const char *sym);
typedef void (*retro_hw_context_reset_t)(void);

struct retro_hw_render_callback {
   enum retro_hw_context_type context_type;
   retro_hw_context_reset_t context_reset;
   retro_hw_get_current_framebuffer_t get_current_framebuffer;
   retro_hw_get_proc_address_t get_proc_address;
   bool depth;   /* deprecated */
   bool stencil; /* deprecated */
   bool bottom_left_origin;
   unsigned version_major;
   unsigned version_minor;
   bool cache_context;
   retro_hw_context_reset_t context_destroy;
   bool debug_context;
};

/* Special value a core passes as `data` to retro_video_refresh_t when
 * hardware-rendering: "the frame is already in the framebuffer you gave me
 * via get_current_framebuffer, go read it yourself" — not a real pixel
 * buffer pointer, must be checked for BEFORE treating `data` as one.
 */
#define RETRO_HW_FRAME_BUFFER_VALID ((void*)-1)

/* retro_log_callback (RETRO_ENVIRONMENT_GET_LOG_INTERFACE) */
enum retro_log_level {
   RETRO_LOG_DEBUG = 0,
   RETRO_LOG_INFO,
   RETRO_LOG_WARN,
   RETRO_LOG_ERROR
};

typedef void (*retro_log_printf_t)(enum retro_log_level level, const char *fmt, ...);

struct retro_log_callback {
   retro_log_printf_t log;
};

/* Callback type definitions */
typedef void (*retro_video_refresh_t)(const void *data, unsigned width, unsigned height, size_t pitch);
typedef void (*retro_audio_sample_t)(int16_t left, int16_t right);
typedef size_t (*retro_audio_sample_batch_t)(const int16_t *data, size_t frames);
typedef void (*retro_input_poll_t)(void);
typedef int16_t (*retro_input_state_t)(unsigned port, unsigned device, unsigned index, unsigned id);
typedef bool (*retro_environment_t)(unsigned cmd, void *data);

/* Function typedefs for core symbols */
typedef void (*retro_init_t)(void);
typedef void (*retro_deinit_t)(void);
typedef unsigned (*retro_api_version_t)(void);
typedef void (*retro_get_system_info_t)(struct retro_system_info *info);
typedef void (*retro_get_system_av_info_t)(struct retro_system_av_info *info);
typedef void (*retro_set_controller_port_device_t)(unsigned port, unsigned device);
typedef void (*retro_reset_t)(void);
typedef void (*retro_run_t)(void);
typedef bool (*retro_load_game_t)(const struct retro_game_info *game);
typedef void (*retro_unload_game_t)(void);
typedef bool (*retro_serialize_t)(void *data, size_t size);
typedef bool (*retro_unserialize_t)(const void *data, size_t size);
typedef size_t (*retro_serialize_size_t)(void);
typedef void (*retro_cheat_reset_t)(void);
typedef void (*retro_cheat_set_t)(unsigned index, bool enabled, const char *code);

typedef void (*retro_set_video_refresh_t)(retro_video_refresh_t);
typedef void (*retro_set_audio_sample_t)(retro_audio_sample_t);
typedef void (*retro_set_audio_sample_batch_t)(retro_audio_sample_batch_t);
typedef void (*retro_set_input_poll_t)(retro_input_poll_t);
typedef void (*retro_set_input_state_t)(retro_input_state_t);
typedef void (*retro_set_environment_t)(retro_environment_t);

#ifdef __cplusplus
}
#endif

#endif /* LIBRETRO_H__ */
