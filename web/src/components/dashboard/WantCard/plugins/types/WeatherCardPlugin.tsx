import React from 'react';
import { Want } from '@/types/want';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

type WeatherCondition = 'sunny' | 'cloudy' | 'rain' | 'snow' | 'storm' | 'fog' | 'default';

interface WeatherConfig {
  gradient: string;
  emoji: string;
  particles: string[];
  textColor: string;
  label: string;
}

const WEATHER_CONFIGS: Record<WeatherCondition, WeatherConfig> = {
  sunny: {
    gradient: 'linear-gradient(160deg, #f7b733 0%, #fc4a1a 40%, #ff6b35 70%, #ffd700 100%)',
    emoji: '☀️',
    particles: ['✦', '✦', '✦', '✦', '✦'],
    textColor: 'text-yellow-900',
    label: '晴れ',
  },
  cloudy: {
    gradient: 'linear-gradient(160deg, #4b6cb7 0%, #6d8cc7 40%, #8fa8d8 70%, #b8cce4 100%)',
    emoji: '☁️',
    particles: ['☁', '☁', '☁', '☁'],
    textColor: 'text-blue-100',
    label: '曇り',
  },
  rain: {
    gradient: 'linear-gradient(160deg, #1a1a2e 0%, #16213e 40%, #0f3460 70%, #1a3a5c 100%)',
    emoji: '🌧️',
    particles: ['|', '|', '|', '|', '|', '|', '|', '|'],
    textColor: 'text-blue-200',
    label: '雨',
  },
  snow: {
    gradient: 'linear-gradient(160deg, #74b9ff 0%, #a29bfe 40%, #dfe6e9 70%, #ffffff 100%)',
    emoji: '❄️',
    particles: ['❄', '❆', '❅', '❄', '❆', '❅'],
    textColor: 'text-blue-900',
    label: '雪',
  },
  storm: {
    gradient: 'linear-gradient(160deg, #0f0c29 0%, #302b63 40%, #24243e 70%, #1a1a2e 100%)',
    emoji: '⛈️',
    particles: ['⚡', '⚡', '⚡'],
    textColor: 'text-purple-200',
    label: '嵐',
  },
  fog: {
    gradient: 'linear-gradient(160deg, #636e72 0%, #b2bec3 40%, #dfe6e9 70%, #f5f6fa 100%)',
    emoji: '🌫️',
    particles: ['~', '~', '~', '~', '~'],
    textColor: 'text-gray-700',
    label: '霧',
  },
  default: {
    gradient: 'linear-gradient(160deg, #667eea 0%, #764ba2 40%, #a29bfe 70%, #c4b5fd 100%)',
    emoji: '🌤️',
    particles: ['·', '·', '·', '·'],
    textColor: 'text-purple-100',
    label: '天気',
  },
};

interface ParticleProps {
  char: string;
  index: number;
  total: number;
  condition: WeatherCondition;
}

const Particle: React.FC<ParticleProps> = ({ char, index, total, condition }) => {
  const frac = index / total;
  const delay = `${(index * 0.4).toFixed(1)}s`;

  // Each condition uses its own animation and position strategy
  const isDrift = condition === 'cloudy' || condition === 'fog';
  const isFlash = condition === 'storm';
  const isFall = condition === 'rain' || condition === 'snow';
  const isPulse = condition === 'sunny' || condition === 'default';

  let animStyle: React.CSSProperties = { position: 'absolute', pointerEvents: 'none', userSelect: 'none' };

  if (isFall) {
    const duration = condition === 'rain' ? '1.2s' : '3s';
    animStyle = {
      ...animStyle,
      left: `${frac * 90 + 5}%`,
      top: '-10%',
      fontSize: condition === 'rain' ? '13px' : '16px',
      animation: `weather-fall ${duration} linear ${delay} infinite`,
      opacity: 0.8,
    };
  } else if (isDrift) {
    // Clouds and fog drift horizontally across the card
    const top = `${frac * 70 + 5}%`;
    const duration = condition === 'cloudy' ? `${5 + index}s` : `${7 + index}s`;
    animStyle = {
      ...animStyle,
      top,
      left: '-20%',
      fontSize: condition === 'cloudy' ? `${14 + index * 2}px` : '12px',
      animation: `weather-drift ${duration} linear ${delay} infinite`,
      opacity: condition === 'cloudy' ? 0.5 : 0.3,
    };
  } else if (isFlash) {
    const left = `${frac * 80 + 10}%`;
    const top = `${(index % 3) * 25 + 5}%`;
    animStyle = {
      ...animStyle,
      left,
      top,
      fontSize: '18px',
      animation: `weather-flash ${2 + index * 0.7}s ease-in-out ${delay} infinite`,
      opacity: 0,
    };
  } else if (isPulse) {
    const left = `${frac * 80 + 10}%`;
    const top = `${(index % 3) * 30 + 5}%`;
    animStyle = {
      ...animStyle,
      left,
      top,
      fontSize: '10px',
      animation: `weather-twinkle ${2.5 + index * 0.5}s ease-in-out ${delay} infinite`,
      opacity: 0,
    };
  }

  return <span style={animStyle}>{char}</span>;
};

const WeatherContentSection: React.FC<WantCardPluginProps> = ({ want, isChild, isControl, isFocused }) => {
  const condition = (GetWeatherCondition(want)) as WeatherCondition;
  const config = WEATHER_CONFIGS[condition] ?? WEATHER_CONFIGS.default;
  const weatherText = (want.state?.current?.weather_text as string) || '';
  const weatherDate = (want.state?.current?.weather_date as string) || '';
  const city = (want.state?.current?.weather_city as string) || (want.spec?.params?.weather_city as string) || 'Tokyo';

  const compact = isChild || (isControl && !isFocused);
  const mt = compact ? 'mt-2' : 'mt-4';

  return (
    <>
      <style>{`
        @keyframes weather-fall {
          0%   { transform: translateY(0) rotate(0deg); opacity: 0; }
          10%  { opacity: 0.8; }
          90%  { opacity: 0.8; }
          100% { transform: translateY(130px) rotate(${condition === 'rain' ? '15deg' : '360deg'}); opacity: 0; }
        }
        @keyframes weather-drift {
          0%   { transform: translateX(0); opacity: 0; }
          10%  { opacity: 0.5; }
          90%  { opacity: 0.5; }
          100% { transform: translateX(140%); opacity: 0; }
        }
        @keyframes weather-flash {
          0%, 100% { opacity: 0; }
          10%, 12% { opacity: 1; }
          11%       { opacity: 0.2; }
        }
        @keyframes weather-twinkle {
          0%, 100% { opacity: 0; transform: scale(0.8); }
          50%       { opacity: 0.7; transform: scale(1.2); }
        }
        @keyframes weather-pulse {
          0%, 100% { opacity: 0.8; }
          50%       { opacity: 1; }
        }
      `}</style>
      <div
        className={`${mt} rounded-xl overflow-hidden relative`}
        style={{
          background: config.gradient,
          minHeight: compact ? '80px' : '120px',
        }}
      >
        {/* Animated particles */}
        <div className="absolute inset-0 overflow-hidden pointer-events-none">
          {config.particles.map((char, i) => (
            <Particle key={i} char={char} index={i} total={config.particles.length} condition={condition} />
          ))}
        </div>

        {/* Content */}
        <div className="relative z-10 p-3 flex flex-col h-full">
          {weatherText ? (
            <>
              <div className="flex items-start justify-between">
                <div>
                  <p
                    className={`text-xs font-semibold uppercase tracking-wide opacity-80 ${config.textColor}`}
                    style={{ textShadow: '0 1px 3px rgba(0,0,0,0.3)' }}
                  >
                    {city}
                  </p>
                  <p
                    className={`text-sm font-medium mt-1 ${config.textColor}`}
                    style={{ textShadow: '0 1px 4px rgba(0,0,0,0.4)', maxWidth: '80%' }}
                  >
                    {weatherText}
                  </p>
                </div>
                <span
                  className="text-3xl"
                  style={{ filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.3))', animation: 'weather-pulse 3s ease-in-out infinite' }}
                >
                  {config.emoji}
                </span>
              </div>
              {weatherDate && !compact && (
                <p
                  className={`text-xs opacity-60 mt-auto pt-2 ${config.textColor}`}
                  style={{ textShadow: '0 1px 2px rgba(0,0,0,0.3)' }}
                >
                  {weatherDate}
                </p>
              )}
            </>
          ) : (
            <div className="flex items-center justify-center h-full py-2">
              <span className="text-2xl opacity-60 animate-pulse">{config.emoji}</span>
              <span className={`ml-2 text-sm opacity-60 ${config.textColor}`} style={{ textShadow: '0 1px 3px rgba(0,0,0,0.3)' }}>
                取得中...
              </span>
            </div>
          )}
        </div>
      </div>
    </>
  );
};

function GetWeatherCondition(want: Want): WeatherCondition {
  const cond = want.state?.current?.weather_condition as string;
  if (cond && cond in WEATHER_CONFIGS) return cond as WeatherCondition;
  // Fallback: infer from weather_text if condition not yet set
  const text = ((want.state?.current?.weather_text as string) || '').toLowerCase();
  if (!text) return 'default';
  if (/thunder|storm|lightning|雷|嵐/.test(text)) return 'storm';
  if (/snow|blizzard|sleet|雪/.test(text)) return 'snow';
  if (/rain|shower|drizzle|雨/.test(text)) return 'rain';
  if (/fog|mist|haze|霧/.test(text)) return 'fog';
  if (/cloud|overcast|曇/.test(text)) return 'cloudy';
  if (/sun|clear|晴|fine/.test(text)) return 'sunny';
  return 'default';
}

registerWantCardPlugin({
  types: ['weather'],
  ContentSection: WeatherContentSection,
});
