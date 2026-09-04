import React, { useState, useEffect } from 'react';
import { Check, X, Eye, EyeOff, AlertTriangle } from 'lucide-react';
import { SecuritySettingProps } from '@/types/security';

const SecuritySetting: React.FC<SecuritySettingProps> = ({
  setting,
  onChange,
  disabled = false,
  error = null,
}) => {
  const [localValue, setLocalValue] = useState(setting.value);
  const [showValue, setShowValue] = useState(!setting.sensitive);
  const [isValid, setIsValid] = useState(true);

  // Validate input on change
  useEffect(() => {
    if (setting.validation && typeof setting.validation === 'function') {
      const validationError = setting.validation(localValue);
      setIsValid(!validationError);
    } else {
      // Built-in validations
      if (setting.type === 'number' || setting.type === 'slider') {
        const num = Number(localValue);
        if (setting.min !== undefined && num < setting.min) setIsValid(false);
        else if (setting.max !== undefined && num > setting.max) setIsValid(false);
        else setIsValid(true);
      }
    }
  }, [localValue, setting]);

  // Handle value change
  const handleChange = (value: any) => {
    setLocalValue(value);

    // For immediate updates (toggles), call onChange right away
    if (setting.type === 'toggle') {
      onChange(value);
    }
  };

  // Handle blur for text-like inputs
  const handleBlur = () => {
    if (setting.type === 'toggle') return;

    if (isValid && localValue !== setting.value) {
      onChange(localValue);
    } else if (!isValid) {
      // Revert to original value on invalid
      setLocalValue(setting.value);
    }
  };

  // Render toggle switch
  const renderToggle = () => {
    const isEnabled = Boolean(localValue);

    return (
      <button
        onClick={() => handleChange(!isEnabled)}
        disabled={disabled}
        className={`
          toggle focus:ring-blue-500
          ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
          ${isEnabled ? 'toggle-on' : 'toggle-off'}
        `}
      >
        <span
          className={`
            toggle-knob
            ${isEnabled ? 'toggle-knob-on' : 'toggle-knob-off'}
          `}
        />
      </button>
    );
  };

  // Render select dropdown
  const renderSelect = () => {
    const options = (setting.options ?? []).map((opt) =>
      typeof opt === 'string' ? { label: opt, value: opt } : opt
    );
    return (
      <select
        value={localValue}
        onChange={(e) => handleChange(e.target.value)}
        disabled={disabled}
        onBlur={handleBlur}
        className={`
          w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500
          ${disabled ? 'bg-gray-100 cursor-not-allowed' : 'bg-white'}
          ${error ? 'border-red-300' : 'border-gray-300'}
        `}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label.charAt(0).toUpperCase() + option.label.slice(1).replace(/_/g, ' ')}
          </option>
        ))}
      </select>
    );
  };

  // Render number input
  const renderNumber = () => (
    <input
      type="number"
      value={localValue}
      onChange={(e) => handleChange(Number(e.target.value))}
      disabled={disabled}
      onBlur={handleBlur}
      min={setting.min}
      max={setting.max}
      step={setting.step}
      className={`
        w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500
        ${disabled ? 'bg-gray-100 cursor-not-allowed' : 'bg-white'}
        ${error ? 'border-red-300' : isValid ? 'border-gray-300' : 'border-red-300'}
      `}
    />
  );

  // Render text input
  const renderText = () => (
    <div className="relative">
      <input
        type={setting.sensitive && !showValue ? 'password' : 'text'}
        value={localValue}
        onChange={(e) => handleChange(e.target.value)}
        disabled={disabled}
        onBlur={handleBlur}
        className={`
          w-full px-3 py-2 pr-10 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500
          ${disabled ? 'bg-gray-100 cursor-not-allowed' : 'bg-white'}
          ${error ? 'border-red-300' : isValid ? 'border-gray-300' : 'border-red-300'}
        `}
      />
      {setting.sensitive && (
        <button
          type="button"
          onClick={() => setShowValue(!showValue)}
          className="absolute right-2 top-1/2 transform -translate-y-1/2 text-gray-400 hover:text-gray-600"
        >
          {showValue ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
        </button>
      )}
    </div>
  );

  // Render slider
  const renderSlider = () => {
    const min = setting.min || 0;
    const max = setting.max || 100;
    const percentage = ((Number(localValue) - min) / (max - min)) * 100;

    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between text-sm text-gray-600">
          <span>{min}</span>
          <span className="font-medium text-gray-900">{localValue}</span>
          <span>{max}</span>
        </div>
        <input
          type="range"
          min={min}
          max={max}
          step={setting.step || 1}
          value={localValue}
          onChange={(e) => handleChange(Number(e.target.value))}
          onMouseUp={handleBlur}
          disabled={disabled}
          className={`
            w-full h-2 bg-gray-200 rounded-lg appearance-none cursor-pointer
            ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
            [&::-webkit-slider-thumb]:appearance-none
            [&::-webkit-slider-thumb]:w-4
            [&::-webkit-slider-thumb]:h-4
            [&::-webkit-slider-thumb]:rounded-full
            [&::-webkit-slider-thumb]:bg-blue-600
            [&::-webkit-slider-thumb]:cursor-pointer
            [&::-moz-range-thumb]:w-4
            [&::-moz-range-thumb]:h-4
            [&::-moz-range-thumb]:rounded-full
            [&::-moz-range-thumb]:bg-blue-600
            [&::-moz-range-thumb]:cursor-pointer
            [&::-moz-range-thumb]:border-0
          `}
          style={{
            background: `linear-gradient(to right, #3B82F6 0%, #3B82F6 ${percentage}%, #E5E7EB ${percentage}%, #E5E7EB 100%)`
          }}
        />
        {setting.step && (
          <p className="text-xs text-gray-500">
            Step: {setting.step} {setting.min && setting.max && `(${setting.min} - ${setting.max})`}
          </p>
        )}
      </div>
    );
  };

  // Render checkbox group
  const renderCheckboxGroup = () => {
    const rawOptions = setting.options ?? [];
    const options: Array<{ label: string; value: string }> = rawOptions.map((opt) =>
      typeof opt === 'string' ? { label: opt, value: opt } : opt
    );

    return (
      <div className="space-y-2">
        {options.map((option) => (
          <label
            key={option.value}
            className={`
              flex items-center gap-2 cursor-pointer p-2 rounded-md
              ${disabled ? 'opacity-50 cursor-not-allowed' : 'hover:bg-gray-50'}
            `}
          >
            <input
              type="checkbox"
              checked={Boolean(localValue[option.value])}
              onChange={(e) => {
                const newValue = {
                  ...localValue,
                  [option.value]: e.target.checked,
                };
                handleChange(newValue);
              }}
              disabled={disabled}
              className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
            />
            <span className="text-sm text-gray-700">{option.label}</span>
          </label>
        ))}
      </div>
    );
  };

  // Render JSON editor
  const renderJSON = () => {
    const [tempValue, setTempValue] = useState(JSON.stringify(localValue, null, 2));
    const [jsonError, setJsonError] = useState<string | null>(null);

    useEffect(() => {
      setTempValue(JSON.stringify(localValue, null, 2));
    }, [localValue]);

    const validateJSON = (value: string) => {
      try {
        const parsed = JSON.parse(value);
        setJsonError(null);
        handleChange(parsed);
      } catch (e) {
        setJsonError('Invalid JSON format');
      }
    };

    return (
      <div className="space-y-2">
        <textarea
          value={tempValue}
          onChange={(e) => {
            setTempValue(e.target.value);
            if (jsonError) setJsonError(null);
          }}
          onBlur={() => validateJSON(tempValue)}
          disabled={disabled}
          rows={8}
          className={`
            w-full px-3 py-2 border rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 font-mono text-sm
            ${disabled ? 'bg-gray-100 cursor-not-allowed' : 'bg-white'}
            ${error || jsonError ? 'border-red-300' : 'border-gray-300'}
          `}
        />
        {jsonError && (
          <div className="flex items-center gap-2 text-sm text-red-600">
            <AlertTriangle className="w-4 h-4" />
            <span>{jsonError}</span>
          </div>
        )}
      </div>
    );
  };

  // Render based on setting type
  const renderControl = () => {
    switch (setting.type) {
      case 'toggle':
        return renderToggle();
      case 'select':
        return renderSelect();
      case 'number':
        return renderNumber();
      case 'text':
        return renderText();
      case 'slider':
        return renderSlider();
      case 'checkbox-group':
        return renderCheckboxGroup();
      case 'json':
        return renderJSON();
      default:
        return <span className="text-gray-500">Unknown setting type</span>;
    }
  };

  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-gray-700">
        {setting.label}
        {setting.required && <span className="ml-1 text-red-500">*</span>}
      </label>

      {renderControl()}

      {/* Validation Status */}
      {localValue !== setting.value && isValid && (
        <div className="flex items-center gap-1 text-xs text-green-600">
          <Check className="w-3 h-3" />
          <span>Changed</span>
        </div>
      )}

      {!isValid && (
        <div className="flex items-center gap-1 text-xs text-red-600">
          <X className="w-3 h-3" />
          <span>Invalid value</span>
        </div>
      )}

      {/* Error message */}
      {error && (
        <div className="flex items-center gap-1 text-xs text-red-600">
          <AlertTriangle className="w-3 h-3" />
          <span>{error}</span>
        </div>
      )}
    </div>
  );
};

export default SecuritySetting;