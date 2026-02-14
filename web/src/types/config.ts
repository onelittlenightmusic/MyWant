export interface ServerConfig {
  port: number;
  host: string;
  debug: boolean;
  header_position: 'top' | 'bottom';
  color_mode: 'light' | 'dark' | 'system';
}
