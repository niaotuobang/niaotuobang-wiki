import * as React from 'react';

interface EmailTemplateProps {
  text: string;
}

export const EmailTemplate = ({ text }: EmailTemplateProps) => (
  <div>
    <h1>Email Template</h1>
    <p>{text}</p>
  </div>
);