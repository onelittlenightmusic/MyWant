import React, { useEffect, useRef } from 'react';
import { EditorView, basicSetup } from 'codemirror';
import { EditorState } from '@codemirror/state';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';

interface YamlEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  height?: string;
  theme?: 'light' | 'dark';
  readOnly?: boolean;
}

export const YamlEditor: React.FC<YamlEditorProps> = ({
  value,
  onChange,
  placeholder = 'Enter YAML configuration...',
  className = '',
  height = '400px',
  theme = 'light',
  readOnly = false
}) => {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView>();

  useEffect(() => {
    if (!editorRef.current) return;

    const extensions = [
      basicSetup,
      yaml(),
      EditorView.theme({
        '&': {
          height: height,
        },
        '.cm-content': {
          padding: '12px',
          fontSize: '14px',
          lineHeight: '1.6',
        },
        '.cm-focused': {
          outline: 'none',
        },
        '.cm-editor': {
          borderRadius: '6px',
          border: '1px solid #d1d5db',
        },
        '.cm-editor.cm-focused': {
          borderColor: '#3b82f6',
          boxShadow: '0 0 0 3px rgba(59, 130, 246, 0.1)',
        },
      }),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          onChange(update.state.doc.toString());
        }
      }),
    ];

    if (theme === 'dark') {
      extensions.push(oneDark);
    }

    if (readOnly) {
      extensions.push(EditorState.readOnly.of(true));
    }

    const state = EditorState.create({
      doc: value,
      extensions,
    });

    const view = new EditorView({
      state,
      parent: editorRef.current,
    });

    viewRef.current = view;

    return () => {
      view.destroy();
    };
  }, [height, theme, readOnly]);

  // Update editor content when value prop changes
  useEffect(() => {
    if (viewRef.current && value !== viewRef.current.state.doc.toString()) {
      viewRef.current.dispatch({
        changes: {
          from: 0,
          to: viewRef.current.state.doc.length,
          insert: value,
        },
      });
    }
  }, [value]);

  return (
    <div className={`yaml-editor ${className}`}>
      <div ref={editorRef} />
    </div>
  );
};