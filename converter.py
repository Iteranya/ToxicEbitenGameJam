#!/usr/bin/env python3
"""
MARKDOWN → HIKARIN RAW LINES (batch conversion, output .py files)
Usage: python3 md2raw.py <file.md or directory>
"""

import sys
import re
from pathlib import Path

def parse(filepath):
    lines_out = []
    with open(filepath, 'r', encoding='utf-8') as f:
        for raw in f:
            line = raw.strip()
            if not line or line.startswith('%%'):
                continue

            # Dialogue: "Speaker: text"
            m = re.match(r'^([\w\s]+?):\s*(.+)', line)
            if m:
                speaker = m.group(1).strip()
                text = m.group(2).strip()
                if speaker.lower() == 'none':
                    lines_out.append(f'vn.say(None, "{text}")')
                else:
                    lines_out.append(f'vn.say("{speaker}", "{text}")')
                continue

            # Image: Char - sprite
            m = re.match(r'^Image:\s*(.+?)\s*-\s*(.+)', line)
            if m:
                char = m.group(1).strip()
                sprite = m.group(2).strip()
                lines_out.append(f'vn.show("{char}", "{sprite}")')
                continue

            # ImageLeft / ImageRight / ImageFull / ImageCustom (same pattern, unquoted char)
            m = re.match(r'^ImageLeft:\s*(.+?)\s*-\s*(.+)', line)
            if m:
                lines_out.append(f'vn.show_left("{m.group(1).strip()}", "{m.group(2).strip()}")')
                continue
            m = re.match(r'^ImageRight:\s*(.+?)\s*-\s*(.+)', line)
            if m:
                lines_out.append(f'vn.show_right("{m.group(1).strip()}", "{m.group(2).strip()}")')
                continue
            m = re.match(r'^ImageFull:\s*(.+?)\s*-\s*(.+)', line)
            if m:
                lines_out.append(f'vn.show_full("{m.group(1).strip()}", "{m.group(2).strip()}")')
                continue
            m = re.match(r'^ImageCustom:\s*(.+?)\s*-\s*(.+?)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)', line)
            if m:
                lines_out.append(f'vn.show_custom("{m.group(1).strip()}", "{m.group(2).strip()}", '
                                 f'{m.group(3)}, {m.group(4)}, {m.group(5)}, {m.group(6)}, {m.group(7)}, {m.group(8)})')
                continue

            # Background: filename
            m = re.match(r'^Background:\s*(.+)', line)
            if m:
                lines_out.append(f'vn.background("{m.group(1).strip()}")')
                continue

            # Music / sound
            m = re.match(r'^Music:\s*(.+)', line)
            if m:
                lines_out.append(f'vn.play_music("{m.group(1).strip()}")')
                continue
            if line.lower() in ('stop music', 'music: stop'):
                lines_out.append('vn.stop_music()')
                continue
            m = re.match(r'^VoiceEffect:\s*(.+)', line)
            if m:
                lines_out.append(f'vn.voice_effect("{m.group(1).strip()}")')
                continue

            # Remove: character [sprite]
            m = re.match(r'^Remove:\s*(.+?)(?:\s+(.+))?\s*$', line)
            if m:
                char = m.group(1).strip()
                sprite = m.group(2).strip() if m.group(2) else None
                if sprite:
                    lines_out.append(f'vn.remove({char}, "{sprite}")')
                else:
                    lines_out.append(f'vn.remove({char})')
                continue

            # Variables
            m = re.match(r'^Set:\s*(\w+)\s*=\s*(.+)', line)
            if m:
                var, val = m.group(1).strip(), m.group(2).strip()
                if val.lower() in ('true', 'false'):
                    lines_out.append(f'vn.setVar("{var}", {val.lower()})')
                else:
                    try:
                        float(val)
                        lines_out.append(f'vn.setVar("{var}", {val})')
                    except ValueError:
                        lines_out.append(f'vn.setVar("{var}", "{val}")')
                continue
            m = re.match(r'^Add:\s*(\w+)\s+(\d+)', line)
            if m:
                lines_out.append(f'vn.addVar("{m.group(1)}", {m.group(2)})')
                continue
            m = re.match(r'^Sub:\s*(\w+)\s+(\d+)', line)
            if m:
                lines_out.append(f'vn.subVar("{m.group(1)}", {m.group(2)})')
                continue
            m = re.match(r'^Mod:\s*(\w+)\s+(\S+)', line)
            if m:
                lines_out.append(f'vn.modVar("{m.group(1)}", "{m.group(2)}")')
                continue

            # Narration (no match above)
            lines_out.append(f'vn.say(None, "{line}")')

    return lines_out

def process_file(input_path, output_path):
    lines = parse(input_path)
    with open(output_path, 'w', encoding='utf-8') as out:
        out.write('\n'.join(lines) + '\n')
    print(f"Written: {output_path}")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python3 md2raw.py <file.md or directory>")
        sys.exit(1)

    target = Path(sys.argv[1])
    if not target.exists():
        print(f"Path does not exist: {target}")
        sys.exit(1)

    if target.is_dir():
        md_files = sorted(target.glob("*.md"))
        if not md_files:
            print(f"No .md files found in {target}")
            sys.exit(0)
        for md_file in md_files:
            out_file = md_file.with_suffix('.py')
            process_file(md_file, out_file)
    else:
        # single file
        out_file = target.with_suffix('.py')
        process_file(target, out_file)